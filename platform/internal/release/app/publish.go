package app

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
	"github.com/felixgeelhaar/glossa/platform/internal/release/delivery"
	"github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// PublishInput is what to publish where.
type PublishInput struct {
	Environment string
	Note        string
}

// Publish builds a release of the project under the environment's
// policy, stores its artifacts, records it and points the environment
// at it. A repeated idemKey returns the first request's release with
// replayed set.
//
// The expensive part — reading the catalog, serializing and uploading —
// happens before the transaction that records the release. Artifacts
// are content-addressed, so uploading one that exists is skipped:
// publishing an unchanged catalog uploads nothing.
func (s *Service) Publish(ctx context.Context, project uuid.UUID, in PublishInput, idemKey string) (domain.Release, bool, error) {
	by, err := s.checkProject(ctx, project, authz.ReleasesPublish)
	if err != nil {
		return domain.Release{}, false, err
	}
	name, err := domain.ParseEnvironmentName(in.Environment)
	if err != nil {
		return domain.Release{}, false, err
	}
	id, keyed, err := idempotentID("release.publish", project.String(), by, idemKey)
	if err != nil {
		return domain.Release{}, false, err
	}
	env, first, err := s.prepare(ctx, project, name, id, keyed)
	if err != nil || first != nil {
		if first != nil {
			return s.replay(*first, in)
		}
		return domain.Release{}, false, err
	}
	built, err := s.build(ctx, project, env.Policy)
	if err != nil {
		return domain.Release{}, false, err
	}
	if built.Stats.NewArtifacts, err = s.upload(ctx, project, built.Artifacts); err != nil {
		return domain.Release{}, false, err
	}
	rel, replayed, err := s.record(ctx, id, project, env, built, in.Note, by)
	if err != nil || replayed {
		if replayed {
			return s.replay(rel, in)
		}
		return domain.Release{}, false, err
	}
	s.syncNow(ctx, project, name)
	return rel, false, nil
}

// prepare ensures the environments exist and returns the target one, or
// the release an earlier request with the same key already recorded.
func (s *Service) prepare(ctx context.Context, project uuid.UUID, name string, id uuid.UUID, keyed bool) (domain.Environment, *domain.Release, error) {
	var (
		env   domain.Environment
		first *domain.Release
	)
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		if keyed {
			r, err := st.Release(ctx, project, id)
			if err == nil {
				first = &r
				return nil
			}
			if !isNotFound(err) {
				return err
			}
		}
		if err := s.ensureDefaults(ctx, st, project); err != nil {
			return err
		}
		var err error
		env, err = st.Environment(ctx, project, name, false)
		return err
	})
	return env, first, err
}

func (s *Service) replay(first domain.Release, in PublishInput) (domain.Release, bool, error) {
	if first.Environment != in.Environment || first.Note != in.Note {
		return domain.Release{}, false, ErrIdempotencyReuse
	}
	return first, true, nil
}

// build is the one build of a release, shared by Publish and
// PreviewPublish: the project's releasable source and the translations
// policy makes eligible, turned into artifacts. It reads; it stores
// nothing.
func (s *Service) build(ctx context.Context, project uuid.UUID, policy domain.Policy) (domain.Built, error) {
	snap, err := s.source.Snapshot(ctx, project, policy.States)
	if err != nil {
		return domain.Built{}, err
	}
	return domain.Build(snap, policy)
}

// missing returns the artifacts storage doesn't have yet.
func (s *Service) missing(ctx context.Context, project uuid.UUID, artifacts []domain.Artifact) ([]domain.Artifact, error) {
	var out []domain.Artifact
	for _, a := range artifacts {
		ok, err := s.objects.Exists(ctx, delivery.ArtifactPath(project.String(), a.Ref.SHA256))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrStorage, err)
		}
		if !ok {
			out = append(out, a)
		}
	}
	return out, nil
}

// upload stores the artifacts storage doesn't have yet and returns how
// many it wrote.
func (s *Service) upload(ctx context.Context, project uuid.UUID, artifacts []domain.Artifact) (int, error) {
	missing, err := s.missing(ctx, project, artifacts)
	if err != nil {
		return 0, err
	}
	for i, a := range missing {
		if err := s.objects.Put(ctx, delivery.ArtifactPath(project.String(), a.Ref.SHA256), a.Body, "application/json"); err != nil {
			return i, fmt.Errorf("%w: %v", ErrStorage, err)
		}
	}
	return len(missing), nil
}

// record commits the release and the pointer move in one transaction.
func (s *Service) record(ctx context.Context, id, project uuid.UUID, target domain.Environment, built domain.Built, note, by string) (domain.Release, bool, error) {
	var (
		rel      domain.Release
		replayed bool
	)
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		envs, err := st.LockProjectEnvironments(ctx, project)
		if err != nil {
			return err
		}
		env, ok := findEnvironment(envs, target.Name)
		if !ok {
			return ErrNotFound
		}
		version, err := st.MaxReleaseVersion(ctx, project)
		if err != nil {
			return err
		}
		rel, err = domain.NewRelease(id, project, version+1, env.Current, env.Name, target.Policy, built, note, by, s.now())
		if err != nil {
			return err
		}
		inserted, err := st.InsertRelease(ctx, rel)
		if err != nil {
			return err
		}
		if !inserted { // the same Idempotency-Key, committed by a concurrent request
			rel, err = st.Release(ctx, project, id)
			replayed = true
			return err
		}
		if err := s.move(ctx, st, &env, rel, domain.ActionPublish, by); err != nil {
			return err
		}
		return st.Publish(ctx, outbox.Event{
			Type: domain.EventPublished, AggregateType: domain.AggregateRelease, AggregateID: rel.ID.String(),
			Payload: domain.Published{
				ReleaseID: rel.ID.String(), ProjectID: project.String(), Version: rel.Version, Environment: env.Name,
				ParentID: optionalID(rel.Parent), ManifestDigest: rel.Digest, Messages: rel.Stats.Messages, By: by,
			},
		})
	})
	return rel, replayed, err
}

func findEnvironment(envs []domain.Environment, name string) (domain.Environment, bool) {
	for _, e := range envs {
		if e.Name == name {
			return e, true
		}
	}
	return domain.Environment{}, false
}

// move points env at rel and appends the deployment to its history.
func (s *Service) move(ctx context.Context, st Store, env *domain.Environment, rel domain.Release, action domain.Action, by string) error {
	previous, expected := env.Current, env.Version
	if !env.Point(rel.ID, s.now()) {
		return nil
	}
	if err := st.UpdateEnvironment(ctx, *env, expected); err != nil {
		return err
	}
	n, err := st.LastDeploymentNumber(ctx, env.ProjectID, env.Name)
	if err != nil {
		return err
	}
	return st.AppendDeployment(ctx, domain.Deployment{
		ProjectID: env.ProjectID, Environment: env.Name, Number: n + 1, ReleaseID: rel.ID, Previous: previous,
		Action: action, By: by, CreatedAt: env.UpdatedAt,
	})
}

func optionalID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

// GetRelease returns one release.
func (s *Service) GetRelease(ctx context.Context, project, id uuid.UUID) (domain.Release, error) {
	if _, err := s.checkProject(ctx, project, authz.ReleasesRead); err != nil {
		return domain.Release{}, err
	}
	var r domain.Release
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		r, err = st.Release(ctx, project, id)
		return err
	})
	return r, err
}

// ListReleases lists a project's releases, newest first.
func (s *Service) ListReleases(ctx context.Context, project uuid.UUID, page pagination.Page) ([]domain.Release, *string, error) {
	if _, err := s.checkProject(ctx, project, authz.ReleasesRead); err != nil {
		return nil, nil, err
	}
	before, err := beforeInt(page.After)
	if err != nil {
		return nil, nil, err
	}
	var rows []domain.Release
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		rows, err = st.Releases(ctx, project, before, page.Limit())
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	items, next := pagination.Trim(rows, page, func(r domain.Release) string { return strconv.Itoa(r.Version) })
	return items, next, nil
}
