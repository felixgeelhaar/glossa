package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// purgePrincipal is the background principal of the retention run.
// Its name is stable: it appears as the actor in logs and audits.
const purgePrincipal = "context.purge"

// Purged says what a purge deleted.
type Purged struct {
	// Builds are the deleted builds (their usages, captures and regions
	// went with them).
	Builds []uuid.UUID
	// OrphanedImages are the images no capture of the project references
	// any more: the caller deletes them from object storage.
	OrphanedImages []domain.Digest
}

// PurgeProject applies the retention policy to a project's builds
// (RFC 0004 §2.3): it keeps the latest builds per application, branch
// and source and every current one, and deletes a closed branch's
// builds after the grace period. Needs catalog.write.
func (s *Service) PurgeProject(ctx context.Context, project uuid.UUID) (Purged, error) {
	if _, err := actor(ctx, authz.CatalogWrite); err != nil {
		return Purged{}, err
	}
	closed, err := s.catalog.ClosedBranches(ctx, project)
	if err != nil {
		return Purged{}, err
	}
	var out Purged
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		all, err := st.BuildSummaries(ctx, project)
		if err != nil {
			return err
		}
		expired := s.retention.Expired(all, closed, s.now())
		if len(expired) == 0 {
			return nil
		}
		images, err := st.BuildImages(ctx, expired)
		if err != nil {
			return err
		}
		if _, err := st.DeleteBuilds(ctx, expired); err != nil {
			return err
		}
		still, err := st.ReferencedImages(ctx, project, images)
		if err != nil {
			return err
		}
		out.Builds = expired
		for _, d := range images {
			if !slices.Contains(still, d) {
				out.OrphanedImages = append(out.OrphanedImages, d)
			}
		}
		return nil
	})
	return out, err
}

// ProjectPurge is one project's part of a retention run.
type ProjectPurge struct {
	ProjectRef
	Purged
}

// Purge runs retention over every project holding builds, each in its
// tenant's scope as the background principal context.purge. It runs
// outside any tenant (the daily job) and needs WithSweeper. A failing
// project doesn't stop the others; their errors are joined.
func (s *Service) Purge(ctx context.Context) ([]ProjectPurge, error) {
	if s.sweeper == nil {
		return nil, errors.New("context: Purge needs a Sweeper")
	}
	targets, err := s.sweeper.ProjectsWithBuilds(ctx)
	if err != nil {
		return nil, err
	}
	var (
		out  []ProjectPurge
		errs []error
	)
	for _, t := range targets {
		bg, err := authz.Background(tenancy.ContextWithTenant(ctx, t.Tenant), purgePrincipal,
			authz.CatalogRead, authz.CatalogWrite)
		if err != nil {
			return out, err
		}
		p, err := s.PurgeProject(bg, t.Project)
		if err != nil {
			s.logger.ErrorContext(ctx, "context: retention", slog.String("tenant_id", t.Tenant.String()),
				slog.String("project_id", t.Project.String()), slog.Any("error", err))
			errs = append(errs, fmt.Errorf("context: purge project %s: %w", t.Project, err))
			continue
		}
		if len(p.Builds) > 0 {
			out = append(out, ProjectPurge{ProjectRef: t, Purged: p})
		}
	}
	return out, errors.Join(errs...)
}
