package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/release/delivery"
	"github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// Catalog's event, as Release reads it (its own payload type: an
// anti-corruption layer over the published JSON).
const catalogProjectDeleted = "catalog.project.deleted"

// Subscribe registers Release's subscribers. Their names are stored with
// each event; never rename them.
//
//   - release.sync_manifest writes an environment's manifest after every
//     pointer move, until storage has it, and removes a destroyed branch
//     environment's.
//   - release.sync_delivery_key writes or removes a key's index object.
//   - release.retire_project stops serving a deleted project: its keys
//     are revoked and its environments and served manifests removed.
//     Releases stay: they are immutable history.
func (s *Service) Subscribe(r *outbox.Registry) error {
	for _, typ := range []string{domain.EventPublished, domain.EventPromoted, domain.EventRolledBack, domain.EventEnvironmentDestroyed} {
		if err := r.Subscribe(typ, "release.sync_manifest", outbox.HandlerFunc(s.handlePointerMoved)); err != nil {
			return err
		}
	}
	for _, typ := range []string{domain.EventDeliveryKeyCreated, domain.EventDeliveryKeyRevoked} {
		if err := r.Subscribe(typ, "release.sync_delivery_key", outbox.HandlerFunc(s.handleKeyChanged)); err != nil {
			return err
		}
	}
	return r.Subscribe(catalogProjectDeleted, "release.retire_project", outbox.HandlerFunc(s.handleProjectDeleted))
}

func parseID(field, s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, outbox.Permanent(fmt.Errorf("release: event %s %q: %w", field, s, err))
	}
	return id, nil
}

func (s *Service) handlePointerMoved(ctx context.Context, d outbox.Delivery) error {
	var e struct {
		ProjectID   string `json:"project_id"`
		Environment string `json:"environment"`
	}
	if err := d.Decode(&e); err != nil {
		return err
	}
	project, err := parseID("project_id", e.ProjectID)
	if err != nil {
		return err
	}
	if !validName(e.Environment) {
		return outbox.Permanent(fmt.Errorf("release: event environment %q", e.Environment))
	}
	return s.SyncEnvironment(ctx, project, e.Environment)
}

func (s *Service) handleKeyChanged(ctx context.Context, d outbox.Delivery) error {
	var e domain.DeliveryKeyChanged
	if err := d.Decode(&e); err != nil {
		return err
	}
	project, err := parseID("project_id", e.ProjectID)
	if err != nil {
		return err
	}
	id, err := parseID("key_id", e.KeyID)
	if err != nil {
		return err
	}
	return s.SyncDeliveryKey(ctx, project, id)
}

// handleProjectDeleted retires a deleted project's delivery: it is
// idempotent, since everything it does is a no-op the second time.
func (s *Service) handleProjectDeleted(ctx context.Context, d outbox.Delivery) error {
	var e struct {
		ProjectID string `json:"project_id"`
	}
	if err := d.Decode(&e); err != nil {
		return err
	}
	project, err := parseID("project_id", e.ProjectID)
	if err != nil {
		return err
	}
	return s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		keys, err := st.ActiveDeliveryKeys(ctx, project)
		if err != nil {
			return err
		}
		for _, k := range keys {
			if err := k.Revoke(systemActor, s.now()); err != nil && !errors.Is(err, domain.ErrKeyRevoked) {
				return err
			}
			if err := st.RevokeDeliveryKey(ctx, k); err != nil {
				return err
			}
			if err := s.syncKey(ctx, st, k); err != nil {
				return err
			}
		}
		names, err := st.DeleteProjectEnvironments(ctx, project)
		if err != nil {
			return err
		}
		for _, name := range names {
			if err := s.remove(ctx, delivery.ManifestPath(project.String(), name)); err != nil {
				return err
			}
		}
		return nil
	})
}
