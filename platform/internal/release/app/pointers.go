package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// Promote points an environment at an existing release of the project
// (staging's release into production). Nothing is rebuilt: the
// release's artifacts are already in storage, and only the
// environment's manifest is written. The environment's policy must
// cover the policy the release was built under, so a preview release
// with drafts never reaches production. Promoting the release an
// environment already serves changes nothing.
func (s *Service) Promote(ctx context.Context, project uuid.UUID, name string, release uuid.UUID) (domain.Environment, error) {
	by, err := s.checkProject(ctx, project, authz.ReleasesPublish)
	if err != nil {
		return domain.Environment{}, err
	}
	if !validName(name) {
		return domain.Environment{}, ErrNotFound
	}
	env, moved, err := s.pointer(ctx, project, name, func(ctx context.Context, st Store, env domain.Environment) (domain.Release, error) {
		rel, err := st.Release(ctx, project, release)
		if isNotFound(err) {
			return domain.Release{}, ErrReleaseNotInProject
		}
		if err != nil {
			return domain.Release{}, err
		}
		if !env.Policy.Covers(rel.Policy) {
			return domain.Release{}, domain.ErrIneligible
		}
		return rel, nil
	}, domain.ActionPromote, domain.EventPromoted, by)
	if err == nil && moved {
		s.syncNow(ctx, project, name)
	}
	return env, err
}

// Rollback points an environment back at a release it served before:
// release, or by default the newest release it served that is older than
// the one it serves now (so rolling back twice walks two steps back).
// Like promote, it only moves the pointer.
func (s *Service) Rollback(ctx context.Context, project uuid.UUID, name string, release *uuid.UUID) (domain.Environment, error) {
	by, err := s.checkProject(ctx, project, authz.ReleasesPublish)
	if err != nil {
		return domain.Environment{}, err
	}
	if !validName(name) {
		return domain.Environment{}, ErrNotFound
	}
	env, moved, err := s.pointer(ctx, project, name, func(ctx context.Context, st Store, env domain.Environment) (domain.Release, error) {
		if env.Current == uuid.Nil {
			return domain.Release{}, domain.ErrNoRollbackTarget
		}
		if release != nil {
			return explicitRollback(ctx, st, env, *release)
		}
		current, err := st.Release(ctx, project, env.Current)
		if err != nil {
			return domain.Release{}, err
		}
		target, err := st.RollbackTarget(ctx, project, name, current.Version)
		if isNotFound(err) {
			return domain.Release{}, domain.ErrNoRollbackTarget
		}
		return target, err
	}, domain.ActionRollback, domain.EventRolledBack, by)
	if err == nil && moved {
		s.syncNow(ctx, project, name)
	}
	return env, err
}

func explicitRollback(ctx context.Context, st Store, env domain.Environment, id uuid.UUID) (domain.Release, error) {
	rel, err := st.Release(ctx, env.ProjectID, id)
	if isNotFound(err) {
		return domain.Release{}, ErrReleaseNotInProject
	}
	if err != nil {
		return domain.Release{}, err
	}
	served, err := st.ServedIn(ctx, env.ProjectID, env.Name, id)
	if err != nil {
		return domain.Release{}, err
	}
	if !served {
		return domain.Release{}, domain.ErrNotInHistory
	}
	return rel, nil
}

// pointer locks the environment, lets choose pick the release to serve,
// and moves the pointer if it changes.
func (s *Service) pointer(ctx context.Context, project uuid.UUID, name string,
	choose func(context.Context, Store, domain.Environment) (domain.Release, error),
	action domain.Action, event, by string,
) (domain.Environment, bool, error) {
	var (
		env   domain.Environment
		moved bool
	)
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		if err := s.ensureDefaults(ctx, st, project); err != nil {
			return err
		}
		var err error
		if env, err = st.Environment(ctx, project, name, true); err != nil {
			return err
		}
		rel, err := choose(ctx, st, env)
		if err != nil {
			return err
		}
		previous := env.Current
		if previous == rel.ID {
			return nil
		}
		if err := s.move(ctx, st, &env, rel, action, by); err != nil {
			return err
		}
		moved = true
		return st.Publish(ctx, outbox.Event{
			Type: event, AggregateType: domain.AggregateRelease, AggregateID: rel.ID.String(),
			Payload: domain.PointerMoved{
				ReleaseID: rel.ID.String(), ProjectID: project.String(), Version: rel.Version, Environment: name,
				PreviousReleaseID: optionalID(previous), By: by,
			},
		})
	})
	return env, moved, err
}
