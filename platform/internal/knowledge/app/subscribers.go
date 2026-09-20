package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/knowledge/domain"
)

// The event contracts Knowledge reads. The payload types are Knowledge's
// own (an anti-corruption layer): it depends on the published JSON, not
// on Localization's or Catalog's Go types.
const (
	localizationTranslationRevised  = "localization.translation.revised"
	localizationTranslationReviewed = "localization.translation.reviewed"
	catalogProjectDeleted           = "catalog.project.deleted"
)

// Subscriber names are stored with each event; never rename them.
const (
	subscriberDeriveTM    = "knowledge.derive_tm"
	subscriberDropProject = "knowledge.drop_project"
)

type translationEvent struct {
	TranslationID string `json:"translation_id"`
	ProjectID     string `json:"project_id"`
}

type projectEvent struct {
	ProjectID string `json:"project_id"`
}

// Subscribe registers Knowledge's subscribers.
//
// Translation memory is derived from both translation events: a review
// approves or takes back an approval (reviewed), and new text either
// arrives approved — a project without required review — or replaces
// approved text (revised).
func (s *Service) Subscribe(r *outbox.Registry) error {
	for _, typ := range []string{localizationTranslationRevised, localizationTranslationReviewed} {
		if err := r.Subscribe(typ, subscriberDeriveTM, outbox.HandlerFunc(s.handleTranslationEvent)); err != nil {
			return err
		}
	}
	return r.Subscribe(catalogProjectDeleted, subscriberDropProject, outbox.HandlerFunc(s.handleProjectDeleted))
}

// handleTranslationEvent brings the translation's TM unit in line with
// the translation's current state, read through Localization's port as
// the background principal knowledge.derive_tm. The event is only a
// trigger: whatever revision it names, the current state decides, and a
// state older than one already applied changes nothing. So duplicates
// and reordering converge.
func (s *Service) handleTranslationEvent(ctx context.Context, d outbox.Delivery) error {
	var e translationEvent
	if err := d.Decode(&e); err != nil {
		return err
	}
	translation, err1 := uuid.Parse(e.TranslationID)
	project, err2 := uuid.Parse(e.ProjectID)
	if err := errors.Join(err1, err2); err != nil {
		return outbox.Permanent(fmt.Errorf("knowledge: translation event %s: %w", d.EventID, err))
	}
	bg, err := authz.Background(ctx, subscriberDeriveTM, authz.TranslationsRead, authz.CatalogRead)
	if err != nil {
		return outbox.Permanent(err)
	}
	cur, err := s.translations.Current(bg, project, translation)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrProjectNotFound) {
		return nil // deleted with its project; knowledge.drop_project erases the rest
	}
	if err != nil {
		return err
	}
	p, _ := authz.From(bg)
	return s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		return s.derive(ctx, st, cur, p.Actor.String())
	})
}

// derive applies domain.Reconcile under the translation's derivation
// lock. A new unit records the text's author; a retirement records by,
// the background process that noticed the change.
func (s *Service) derive(ctx context.Context, st Store, cur CurrentTranslation, by string) error {
	now := s.now()
	if err := st.EnsureDerivation(ctx, cur.TranslationID, cur.ProjectID, now); err != nil {
		return err
	}
	applied, err := st.LockDerivation(ctx, cur.TranslationID)
	if err != nil {
		return err
	}
	if applied >= cur.Revision {
		return nil
	}
	active, err := st.LockActiveUnit(ctx, cur.TranslationID)
	if err != nil {
		return err
	}
	d, err := domain.Reconcile(active, cur.ApprovedText, cur.Approved, now)
	if err != nil {
		return outbox.Permanent(err)
	}
	if d.Retire != "" {
		if err := active.Retire(d.Retire, by, now); err != nil {
			return err
		}
		if err := st.RetireUnit(ctx, *active); err != nil {
			return err
		}
	}
	if d.Touch {
		if err := st.TouchUnit(ctx, active.ID, cur.Revision, now); err != nil {
			return err
		}
	}
	if d.Create != nil {
		if err := st.InsertUnit(ctx, *d.Create); err != nil {
			return err
		}
	}
	return st.SetDerivation(ctx, cur.TranslationID, cur.Revision, now)
}

// handleProjectDeleted erases a deleted project's knowledge: its TM
// units, concepts, style guides and their history.
func (s *Service) handleProjectDeleted(ctx context.Context, d outbox.Delivery) error {
	var e projectEvent
	if err := d.Decode(&e); err != nil {
		return err
	}
	id, err := uuid.Parse(e.ProjectID)
	if err != nil {
		return outbox.Permanent(fmt.Errorf("knowledge: project event with project_id %q", e.ProjectID))
	}
	return s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		return st.DeleteProjectData(ctx, id)
	})
}
