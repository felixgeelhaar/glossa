package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// errNoImages means the service was built without WithImages.
var errNoImages = errors.New("context: capture images need object storage (WithImages)")

// ImageParts yields a capture upload's image parts, in the order they
// arrive after the manifest.
type ImageParts interface {
	// Next returns the next part's name and body; the body is valid until
	// the next call. It returns io.EOF after the last part.
	Next() (name string, body io.Reader, err error)
}

// IngestCaptures is the input of IngestCaptures.
type IngestCaptures struct {
	Project uuid.UUID
	// Manifest is the glossa.captures/v1 manifest's bytes.
	Manifest []byte
	// Images are the upload's image parts, each named by the lowercase
	// hex SHA-256 of its bytes as the manifest references it.
	Images ImageParts
}

// CapturesIngested says what IngestCaptures stored.
type CapturesIngested struct {
	// Build is the capture build (source capture).
	Build domain.Build
	// Captures counts the build's captures.
	Captures int
	// ImagesStored counts the images written to object storage;
	// ImagesDeduplicated those whose pixels were stored already. Both
	// are 0 on a replay: its images aren't read.
	ImagesStored       int
	ImagesDeduplicated int
	// UnknownKeys are the region keys the catalog didn't know, in order
	// (stored with a null message ID, like usages).
	UnknownKeys []string
	// Replayed is true when the same manifest was uploaded before for
	// the application and commit: nothing changed and Build is the first
	// upload's.
	Replayed bool
}

// IngestCaptures stores a capture upload (RFC 0004 §3.2–§3.3): a build
// of source capture with the manifest's captures and their regions,
// region keys resolved to message IDs. Every image part is checked
// against its name and its manifest entry, re-encoded and stored
// content-addressed, once per project. The upload is idempotent by
// application, commit and manifest digest: a repeat returns the first
// build without reading its images. Rate-limited per tenant with the
// usage uploads (ErrRateLimited). Needs catalog.write.
func (s *Service) IngestCaptures(ctx context.Context, in IngestCaptures) (CapturesIngested, error) {
	by, err := actor(ctx, authz.CatalogWrite)
	if err != nil {
		return CapturesIngested{}, err
	}
	if s.objects == nil || s.images == nil {
		return CapturesIngested{}, errNoImages
	}
	if err := s.allowUpload(ctx); err != nil {
		return CapturesIngested{}, err
	}
	up, err := domain.ParseCaptures(in.Manifest)
	if err != nil {
		return CapturesIngested{}, err
	}
	b, err := s.captureBuild(ctx, in.Project, up, by)
	if err != nil {
		return CapturesIngested{}, err
	}
	if first, found, err := s.capturesUploaded(ctx, b); err != nil || found {
		s.recordCaptures(ctx, first, 0, 0)
		return first, err
	}
	images, err := s.receiveImages(ctx, up, in.Images)
	defer closeImages(images)
	if err != nil {
		return CapturesIngested{}, err
	}
	stored, deduplicated, bytes, err := s.storeImages(ctx, in.Project, images)
	if err != nil {
		return CapturesIngested{}, err
	}
	captures, unknown, err := s.newCaptures(ctx, b, up, images)
	if err != nil {
		return CapturesIngested{}, err
	}
	out, err := s.insertCaptures(ctx, b, captures, unknown)
	if err != nil {
		return CapturesIngested{}, err
	}
	if !out.Replayed {
		out.ImagesStored, out.ImagesDeduplicated = stored, deduplicated
	}
	s.recordCaptures(ctx, out, regionCount(captures), bytes)
	return out, nil
}

func (s *Service) captureBuild(ctx context.Context, project uuid.UUID, up domain.CaptureUpload, by string) (domain.Build, error) {
	defaultBranch, err := s.catalog.DefaultBranch(ctx, project)
	if err != nil {
		return domain.Build{}, err
	}
	application, err := s.catalog.Application(ctx, project, up.Application)
	if err != nil {
		return domain.Build{}, err
	}
	return domain.NewBuild(project, application, up.Upload, domain.SourceCapture, up.Branch.String() == defaultBranch, by, s.now())
}

func (s *Service) recordCaptures(ctx context.Context, out CapturesIngested, regions int, bytes int64) {
	if out.Build.ID == uuid.Nil {
		return
	}
	s.metrics.BuildIngested(domain.SourceCapture, 0, 0, out.Replayed)
	if !out.Replayed {
		t, _ := tenancy.FromContext(ctx)
		s.metrics.CapturesIngested(t, out.Captures, regions, out.ImagesStored, out.ImagesDeduplicated, bytes)
	}
}

func regionCount(cs []domain.Capture) int {
	n := 0
	for _, c := range cs {
		n += len(c.Regions)
	}
	return n
}

// capturesUploaded finds an earlier ingest of b's manifest.
func (s *Service) capturesUploaded(ctx context.Context, b domain.Build) (CapturesIngested, bool, error) {
	var out CapturesIngested
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		out, err = replayCaptures(ctx, st, b)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return CapturesIngested{}, false, nil
	}
	return out, err == nil, err
}

func replayCaptures(ctx context.Context, st Store, b domain.Build) (CapturesIngested, error) {
	first, err := st.BuildByUpload(ctx, b.ApplicationID, b.Commit, b.Source, b.Digest)
	if err != nil {
		return CapturesIngested{}, err
	}
	n, err := st.CountCaptures(ctx, first.ID)
	if err != nil {
		return CapturesIngested{}, err
	}
	unknown, err := st.UnknownRegionKeys(ctx, first.ID)
	if unknown == nil {
		unknown = []string{}
	}
	return CapturesIngested{Build: first, Captures: n, UnknownKeys: unknown, Replayed: true}, err
}

// receiveImages reads every image part: each names an image of the
// manifest, once, and is the PNG its entry describes. The result maps
// the uploaded digest to the re-encoded image; the caller closes them,
// on failure too.
func (s *Service) receiveImages(ctx context.Context, up domain.CaptureUpload, parts ImageParts) (map[domain.Digest]NormalizedImage, error) {
	want := map[domain.Digest]domain.Image{}
	for _, img := range up.Parts() {
		want[img.Digest] = img
	}
	got := map[domain.Digest]NormalizedImage{}
	for {
		name, body, err := parts.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return got, err
		}
		d, err := domain.ParseDigest(name)
		entry, referenced := want[d]
		switch {
		case err != nil || !referenced:
			return got, fmt.Errorf("%w: part %q is no image of the manifest", domain.ErrInvalidCaptures, name)
		case got[d] != nil:
			return got, fmt.Errorf("%w: image %s is uploaded twice", domain.ErrInvalidCaptures, d)
		}
		img, err := s.images.Normalize(ctx, body, d)
		if err != nil {
			return got, err
		}
		got[d] = img
		if w, h := img.Image().Width, img.Image().Height; w != entry.Width || h != entry.Height {
			return got, fmt.Errorf("%w: image %s is %d×%d pixels; the manifest says %d×%d", domain.ErrInvalidImage, d, w, h,
				entry.Width, entry.Height)
		}
	}
	for _, img := range up.Parts() {
		if got[img.Digest] == nil {
			return got, fmt.Errorf("%w: no part holds image %s", domain.ErrInvalidCaptures, img.Digest)
		}
	}
	return got, nil
}

func closeImages(images map[domain.Digest]NormalizedImage) {
	for _, img := range images {
		_ = img.Close()
	}
}

// storeImages writes the re-encoded images to object storage, content
// addressed; an image whose pixels are stored already is not written
// again. It returns how many were written and deduplicated, and the
// bytes written.
func (s *Service) storeImages(ctx context.Context, project uuid.UUID, images map[domain.Digest]NormalizedImage) (stored, deduplicated int, bytes int64, err error) {
	t, _ := tenancy.FromContext(ctx)
	done := map[domain.Digest]bool{}
	for _, part := range sortedParts(images) {
		img := images[part]
		d := img.Image().Digest
		key := domain.ImageKey(t, project, d)
		exists := done[d]
		if !exists {
			if exists, err = s.objects.Exists(ctx, key); err != nil {
				return 0, 0, 0, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
			}
		}
		done[d] = true
		if exists {
			deduplicated++
			continue
		}
		n, err := s.putImage(ctx, key, img)
		if err != nil {
			return 0, 0, 0, err
		}
		stored++
		bytes += n
	}
	return stored, deduplicated, bytes, nil
}

func (s *Service) putImage(ctx context.Context, key string, img NormalizedImage) (int64, error) {
	r, err := img.Open()
	if err != nil {
		return 0, err
	}
	defer func() { _ = r.Close() }()
	n, err := s.objects.PutStream(ctx, key, r, "image/png")
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	return n, nil
}

// sortedParts orders the parts by digest, so storing is deterministic.
func sortedParts(images map[domain.Digest]NormalizedImage) []domain.Digest {
	out := make([]domain.Digest, 0, len(images))
	for d := range images {
		out = append(out, d)
	}
	slices.Sort(out)
	return out
}

// newCaptures builds the captures of b: each on its stored image, with
// its region keys resolved to message IDs. It returns the keys the
// catalog didn't know, in order.
func (s *Service) newCaptures(ctx context.Context, b domain.Build, up domain.CaptureUpload, images map[domain.Digest]NormalizedImage) ([]domain.Capture, []string, error) {
	ids, err := s.catalog.MessageIDs(ctx, b.ProjectID, up.Keys())
	if err != nil {
		return nil, nil, err
	}
	out := make([]domain.Capture, len(up.Captures))
	unknown := map[string]bool{}
	for i, in := range up.Captures {
		in.Image = images[in.Image.Digest].Image()
		in.Regions = slices.Clone(in.Regions)
		c, err := domain.NewCapture(b.ProjectID, b.ID, in, b.CreatedBy, b.CreatedAt)
		if err != nil {
			return nil, nil, err
		}
		domain.ResolveRegions(c.Regions, ids)
		for _, r := range c.Regions {
			if r.MessageID == nil {
				unknown[r.Key] = true
			}
		}
		out[i] = c
	}
	keys := make([]string, 0, len(unknown))
	for k := range unknown {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return out, keys, nil
}

// insertCaptures stores b with its captures in one unit of work and
// publishes context.build.ingested and a context.capture.ingested per
// capture. A concurrent upload of the same manifest that won makes this
// one its replay.
func (s *Service) insertCaptures(ctx context.Context, b domain.Build, captures []domain.Capture, unknown []string) (CapturesIngested, error) {
	var out CapturesIngested
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		inserted, err := st.InsertBuild(ctx, b)
		if err != nil {
			return err
		}
		if !inserted {
			out, err = replayCaptures(ctx, st, b)
			return err
		}
		for _, c := range captures {
			ok, err := st.InsertCapture(ctx, c)
			if err != nil {
				return err
			}
			if !ok { // the manifest holds each shot once; a new build none
				return ErrCaptureConflict
			}
			if err := st.Publish(ctx, outbox.Event{
				Type: domain.EventCaptureIngested, AggregateType: domain.AggregateCapture, AggregateID: c.ID.String(),
				Payload: domain.CaptureIngestedOf(c, b), OccurredAt: c.CreatedAt,
			}); err != nil {
				return err
			}
		}
		out = CapturesIngested{Build: b, Captures: len(captures), UnknownKeys: unknown}
		return st.Publish(ctx, outbox.Event{
			Type: domain.EventBuildIngested, AggregateType: domain.AggregateBuild, AggregateID: b.ID.String(),
			Payload: domain.BuildIngestedOf(b, 0), OccurredAt: b.CreatedAt,
		})
	})
	return out, err
}
