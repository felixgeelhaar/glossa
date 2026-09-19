package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
)

// The event contracts Context reads. The payload types are Context's
// own (an anti-corruption layer): it depends on the published JSON, not
// on Catalog's Go types.
const (
	catalogProjectDeleted     = "catalog.project.deleted"
	catalogApplicationDeleted = "catalog.application.deleted"
)

// Subscriber names are stored with each event; never rename them.
const (
	subscriberDropProject     = "context.drop_project"
	subscriberDropApplication = "context.drop_application"
	// subscriberMeasureCoverage is also the background principal that
	// reads the catalog for it.
	subscriberMeasureCoverage = "context.measure_coverage"
)

type catalogEvent struct {
	ProjectID     string `json:"project_id"`
	ApplicationID string `json:"application_id"`
}

// Subscribe registers Context's subscribers: a deleted project or
// application takes its builds, usages, captures and regions with it,
// and a default-branch build updates the project's context coverage.
func (s *Service) Subscribe(r *outbox.Registry) error {
	if err := r.Subscribe(catalogProjectDeleted, subscriberDropProject, outbox.HandlerFunc(s.handleProjectDeleted)); err != nil {
		return err
	}
	if err := r.Subscribe(domain.EventBuildIngested, subscriberMeasureCoverage, outbox.HandlerFunc(s.handleBuildIngested)); err != nil {
		return err
	}
	return r.Subscribe(catalogApplicationDeleted, subscriberDropApplication, outbox.HandlerFunc(s.handleApplicationDeleted))
}

// handleBuildIngested measures the project's context coverage (RFC
// 0004 §11) after a default-branch build: the share of its active
// messages with a current usage. Measuring twice is harmless.
func (s *Service) handleBuildIngested(ctx context.Context, d outbox.Delivery) error {
	var e domain.BuildIngested
	if err := d.Decode(&e); err != nil {
		return err
	}
	if !e.OnDefaultBranch {
		return nil
	}
	project, err := uuid.Parse(e.ProjectID)
	if err != nil {
		return outbox.Permanent(fmt.Errorf("context: build event with project_id %q", e.ProjectID))
	}
	bg, err := authz.Background(ctx, subscriberMeasureCoverage, authz.CatalogRead)
	if err != nil {
		return err
	}
	_, err = s.UnusedMessages(bg, project, "")
	if errors.Is(err, ErrProjectNotFound) {
		return nil // deleted since: nothing to measure
	}
	return err
}

// handleProjectDeleted erases a deleted project's context. Deleting
// what is already gone changes nothing, so redelivery is harmless.
func (s *Service) handleProjectDeleted(ctx context.Context, d outbox.Delivery) error {
	var e catalogEvent
	if err := d.Decode(&e); err != nil {
		return err
	}
	project, err := uuid.Parse(e.ProjectID)
	if err != nil {
		return outbox.Permanent(fmt.Errorf("context: project event with project_id %q", e.ProjectID))
	}
	return s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		return st.DeleteProjectData(ctx, project)
	})
}

// handleApplicationDeleted erases a deleted application's builds.
func (s *Service) handleApplicationDeleted(ctx context.Context, d outbox.Delivery) error {
	var e catalogEvent
	if err := d.Decode(&e); err != nil {
		return err
	}
	project, err1 := uuid.Parse(e.ProjectID)
	application, err2 := uuid.Parse(e.ApplicationID)
	if err := errors.Join(err1, err2); err != nil {
		return outbox.Permanent(fmt.Errorf("context: application event %s: %w", d.EventID, err))
	}
	return s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		return st.DeleteApplicationData(ctx, project, application)
	})
}
