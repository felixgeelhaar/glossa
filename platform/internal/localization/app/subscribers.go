package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/localization/domain"
)

// Catalog's event contract, as Localization reads it. The payload types
// are Localization's own (an anti-corruption layer): it depends on the
// published JSON, not on Catalog's Go types.
const (
	catalogProjectCreated = "catalog.project.created"
	catalogProjectDeleted = "catalog.project.deleted"
)

// catalogMessageEvents all carry a message snapshot.
var catalogMessageEvents = []string{
	"catalog.message.created",
	"catalog.message.source_revised",
	"catalog.message.updated",
	"catalog.message.renamed",
	"catalog.message.obsoleted",
	"catalog.message.reactivated",
	"catalog.message.activated",
	"catalog.message.proposed",
}

type messageSnapshot struct {
	MessageID      string `json:"message_id"`
	ProjectID      string `json:"project_id"`
	Key            string `json:"key"`
	Namespace      string `json:"namespace"`
	State          string `json:"state"`
	SourceRevision int    `json:"source_revision"`
	Version        int    `json:"version"`
}

type projectEvent struct {
	ProjectID    string `json:"project_id"`
	SourceLocale string `json:"source_locale"`
	By           string `json:"by"`
}

// Subscribe registers Localization's subscribers. Their names are stored
// with each event; never rename them.
func (s *Service) Subscribe(r *outbox.Registry) error {
	for _, typ := range catalogMessageEvents {
		if err := r.Subscribe(typ, "localization.track_message", outbox.HandlerFunc(s.handleMessageEvent)); err != nil {
			return err
		}
	}
	if err := r.Subscribe(catalogProjectCreated, "localization.add_source_locale", outbox.HandlerFunc(s.handleProjectCreated)); err != nil {
		return err
	}
	return r.Subscribe(catalogProjectDeleted, "localization.drop_project", outbox.HandlerFunc(s.handleProjectDeleted))
}

// handleMessageEvent keeps the message projection current. It is
// idempotent and order-independent: the snapshot with the highest
// version wins. When the source revision advances, the translations it
// leaves behind are now outdated, and each gets a
// localization.translation.outdated event.
func (s *Service) handleMessageEvent(ctx context.Context, d outbox.Delivery) error {
	var e struct {
		Message messageSnapshot `json:"message"`
	}
	if err := d.Decode(&e); err != nil {
		return err
	}
	state, err := e.Message.state(s.now)
	if err != nil {
		return outbox.Permanent(err)
	}
	return s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		return s.applyMessageState(ctx, st, state, bcp47.Tag{})
	})
}

// ProjectMessages brings the message projection up to date with
// messages a Catalog write is changing, inside that write's transaction
// (Catalog's bulk upsert, through an in-process port): the listings'
// missing_in and outdated_in agree with Catalog at commit, and the
// translations a new source revision leaves behind are announced in the
// same commit. The outbox subscriber stays as the idempotent catch-up —
// it finds the projection at the snapshot's version and changes nothing.
// Needs catalog.write: it is the writer's own change.
func (s *Service) ProjectMessages(ctx context.Context, ms []MessageState) error {
	if err := authz.Require(ctx, authz.CatalogWrite); err != nil {
		return err
	}
	if len(ms) == 0 {
		return nil
	}
	return s.tx.InCurrent(ctx, func(ctx context.Context, st Store) error {
		for _, m := range ms {
			if err := s.applyMessageState(ctx, st, m, bcp47.Tag{}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m messageSnapshot) state(now func() time.Time) (MessageState, error) {
	id, err := uuid.Parse(m.MessageID)
	if err != nil {
		return MessageState{}, fmt.Errorf("localization: message event with message_id %q", m.MessageID)
	}
	project, err := uuid.Parse(m.ProjectID)
	if err != nil {
		return MessageState{}, fmt.Errorf("localization: message event with project_id %q", m.ProjectID)
	}
	if m.Version < 1 || m.SourceRevision < 1 {
		return MessageState{}, fmt.Errorf("localization: message event for %s without version or revision", id)
	}
	return MessageState{
		MessageID: id, ProjectID: project, Key: m.Key, Namespace: m.Namespace, State: m.State,
		SourceRevision: m.SourceRevision, Version: m.Version, UpdatedAt: now(),
	}, nil
}

func stateOf(m SourceMessage) MessageState {
	return MessageState{
		MessageID: m.ID, ProjectID: m.ProjectID, Key: m.Key, Namespace: m.Namespace, State: m.State,
		SourceRevision: m.Revision, Version: m.Version,
	}
}

// applyMessageState stores a message snapshot unless a newer one is
// stored, and announces the translations a source revision made
// outdated — except in `except`, whose translation the caller is about
// to bring current.
func (s *Service) applyMessageState(ctx context.Context, st Store, m MessageState, except bcp47.Tag) error {
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = s.now()
	}
	cur, found, err := st.LockMessageState(ctx, m.MessageID)
	if err != nil {
		return err
	}
	if found && cur.Version >= m.Version {
		return nil
	}
	if err := st.SaveMessageState(ctx, m); err != nil {
		return err
	}
	if !found || m.SourceRevision <= cur.SourceRevision {
		return nil
	}
	outdated, err := st.NewlyOutdated(ctx, m.MessageID, cur.SourceRevision, m.SourceRevision)
	if err != nil {
		return err
	}
	for _, t := range outdated {
		if t.Locale == except {
			continue
		}
		if err := st.Publish(ctx, outbox.Event{
			Type: domain.EventTranslationOutdated, AggregateType: domain.AggregateTranslation, AggregateID: t.ID.String(),
			Payload: domain.TranslationOutdated{
				TranslationID: t.ID.String(), ProjectID: t.ProjectID.String(), MessageID: t.MessageID.String(),
				Locale: t.Locale.String(), SourceRevision: t.SourceRevision, CurrentSourceRevision: m.SourceRevision,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

// handleProjectCreated records the new project's source locale.
func (s *Service) handleProjectCreated(ctx context.Context, d outbox.Delivery) error {
	var e projectEvent
	if err := d.Decode(&e); err != nil {
		return err
	}
	id, err := uuid.Parse(e.ProjectID)
	if err != nil {
		return outbox.Permanent(fmt.Errorf("localization: project event with project_id %q", e.ProjectID))
	}
	source, err := bcp47.Parse(e.SourceLocale)
	if err != nil {
		return outbox.Permanent(err)
	}
	return s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		return s.ensureSourceLocale(ctx, st, ProjectInfo{ID: id, SourceLocale: source, DefaultSyntax: mfcontent.MF1}, e.By)
	})
}

// handleProjectDeleted drops everything Localization holds for a
// deleted project.
func (s *Service) handleProjectDeleted(ctx context.Context, d outbox.Delivery) error {
	var e projectEvent
	if err := d.Decode(&e); err != nil {
		return err
	}
	id, err := uuid.Parse(e.ProjectID)
	if err != nil {
		return outbox.Permanent(fmt.Errorf("localization: project event with project_id %q", e.ProjectID))
	}
	return s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		return st.DeleteProjectData(ctx, id)
	})
}
