package app

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
)

// IngestUsages is the input of IngestUsages.
type IngestUsages struct {
	Project uuid.UUID
	// Source is the collector: plugin, extract, runtime or capture.
	Source string
	// DefaultBranch is the repository's default branch, as the uploader
	// knows it: a build of that branch is what every view of the
	// current usages falls back to.
	DefaultBranch string
	// Document is the glossa.usages/v1 document's bytes; its SHA-256 is
	// the build's digest.
	Document []byte
}

// Ingested says what IngestUsages stored.
type Ingested struct {
	Build domain.Build
	// UnknownKeys counts usages whose key the catalog doesn't know
	// (stored with a null message ID; PR checks report them).
	UnknownKeys int
	// Replayed is true when the same upload was ingested before: nothing
	// changed and Build is the first upload's.
	Replayed bool
}

// IngestUsages stores a build's usages with their keys resolved to
// message IDs, and publishes context.build.ingested (RFC 0004 §2.2).
// Uploading the same document again for the same application, commit
// and source is a no-op. Needs catalog.write.
func (s *Service) IngestUsages(ctx context.Context, in IngestUsages) (Ingested, error) {
	by, err := actor(ctx, authz.CatalogWrite)
	if err != nil {
		return Ingested{}, err
	}
	source, err := domain.ParseSource(in.Source)
	if err != nil {
		return Ingested{}, err
	}
	defaultBranch, err := domain.ParseBranch(in.DefaultBranch)
	if err != nil {
		return Ingested{}, err
	}
	up, err := domain.ParseUpload(in.Document)
	if err != nil {
		return Ingested{}, err
	}
	application, err := s.catalog.Application(ctx, in.Project, up.Application)
	if err != nil {
		return Ingested{}, err
	}
	b, err := domain.NewBuild(in.Project, application, up, source, up.Branch == defaultBranch, by, s.now())
	if err != nil {
		return Ingested{}, err
	}
	if first, found, err := s.uploaded(ctx, b); err != nil || found {
		return first, err
	}
	// Keys are resolved before the unit of work: Catalog's port runs its
	// own transaction.
	ids, err := s.catalog.MessageIDs(ctx, in.Project, domain.UsageKeys(up.Usages))
	if err != nil {
		return Ingested{}, err
	}
	unknown := domain.ResolveUsages(up.Usages, ids)
	var out Ingested
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		inserted, err := st.InsertBuild(ctx, b)
		if err != nil {
			return err
		}
		if !inserted { // a concurrent upload of the same document won
			out, err = replay(ctx, st, b)
			return err
		}
		if err := st.InsertUsages(ctx, b.ID, up.Usages); err != nil {
			return err
		}
		out = Ingested{Build: b, UnknownKeys: unknown}
		return st.Publish(ctx, outbox.Event{
			Type: domain.EventBuildIngested, AggregateType: domain.AggregateBuild, AggregateID: b.ID.String(),
			Payload: domain.BuildIngestedOf(b, unknown), OccurredAt: b.CreatedAt,
		})
	})
	return out, err
}

// uploaded finds an earlier ingest of b's upload.
func (s *Service) uploaded(ctx context.Context, b domain.Build) (Ingested, bool, error) {
	var out Ingested
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		out, err = replay(ctx, st, b)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return Ingested{}, false, nil
	}
	return out, err == nil, err
}

func replay(ctx context.Context, st Store, b domain.Build) (Ingested, error) {
	first, err := st.BuildByUpload(ctx, b.ApplicationID, b.Commit, b.Source, b.Digest)
	if err != nil {
		return Ingested{}, err
	}
	unknown, err := st.CountUnknownKeys(ctx, first.ID)
	return Ingested{Build: first, UnknownKeys: unknown, Replayed: true}, err
}

// IngestCapture is the input of IngestCapture.
type IngestCapture struct {
	Project uuid.UUID
	Build   uuid.UUID
	Capture domain.CaptureInput
}

// CaptureStored says what IngestCapture stored.
type CaptureStored struct {
	Capture domain.Capture
	// UnknownKeys counts regions whose key the catalog doesn't know.
	UnknownKeys int
	// Replayed is true when the build already held this exact capture.
	Replayed bool
}

// IngestCapture adds a capture and its regions to a build of the
// project, with region keys resolved to message IDs, and publishes
// context.capture.ingested (RFC 0004 §3.2). The image is stored by the
// caller first (content-addressed by its digest). Uploading the same
// capture again is a no-op; a different one of the same route, viewport
// and locale is ErrCaptureConflict. Needs catalog.write.
func (s *Service) IngestCapture(ctx context.Context, in IngestCapture) (CaptureStored, error) {
	by, err := actor(ctx, authz.CatalogWrite)
	if err != nil {
		return CaptureStored{}, err
	}
	c, err := domain.NewCapture(in.Project, in.Build, in.Capture, by, s.now())
	if err != nil {
		return CaptureStored{}, err
	}
	ids, err := s.catalog.MessageIDs(ctx, in.Project, domain.RegionKeys(c.Regions))
	if err != nil {
		return CaptureStored{}, err
	}
	unknown := domain.ResolveRegions(c.Regions, ids)
	var out CaptureStored
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		b, err := s.buildOf(ctx, st, in.Project, in.Build)
		if err != nil {
			return err
		}
		if err := st.LockBuildCaptures(ctx, b.ID); err != nil {
			return err
		}
		if out, err = repeatedCapture(ctx, st, c); err == nil || !errors.Is(err, ErrNotFound) {
			return err
		}
		n, err := st.CountCaptures(ctx, b.ID)
		if err != nil {
			return err
		}
		if err := domain.CheckCaptureLimit(n); err != nil {
			return err
		}
		if _, err := st.InsertCapture(ctx, c); err != nil {
			return err
		}
		out = CaptureStored{Capture: c, UnknownKeys: unknown}
		return st.Publish(ctx, outbox.Event{
			Type: domain.EventCaptureIngested, AggregateType: domain.AggregateCapture, AggregateID: c.ID.String(),
			Payload: domain.CaptureIngestedOf(c, b), OccurredAt: c.CreatedAt,
		})
	})
	return out, err
}

// buildOf returns the project's build id.
func (s *Service) buildOf(ctx context.Context, st Store, project, id uuid.UUID) (domain.Build, error) {
	b, err := st.Build(ctx, id)
	if errors.Is(err, ErrNotFound) || (err == nil && b.ProjectID != project) {
		return domain.Build{}, ErrBuildNotFound
	}
	return b, err
}

// repeatedCapture returns the build's stored capture of c's shot:
// ErrNotFound if there is none, ErrCaptureConflict if it differs.
func repeatedCapture(ctx context.Context, st Store, c domain.Capture) (CaptureStored, error) {
	first, err := st.CaptureByShot(ctx, c.BuildID, c.Route, c.Viewport, c.Locale)
	if err != nil {
		return CaptureStored{}, err
	}
	if !first.SameShot(c) {
		return CaptureStored{}, ErrCaptureConflict
	}
	unknown := 0
	for _, r := range first.Regions {
		if r.MessageID == nil {
			unknown++
		}
	}
	return CaptureStored{Capture: first, UnknownKeys: unknown, Replayed: true}, nil
}
