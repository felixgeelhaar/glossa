// Package sqlcadapter implements the domain Repository ports against
// the sqlc-generated Queries struct. Each adapter is a thin shim —
// no business rules, just mapping between domain VOs and sqlc params
// / rows.
package sqlcadapter

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/felixgeelhaar/glossa/apps/api/internal/db"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

// ProjectRepo is the sqlc-backed Repository for projects.
type ProjectRepo struct {
	q *db.Queries
}

// NewProjectRepo wires the repo against a sqlc Queries.
func NewProjectRepo(q *db.Queries) *ProjectRepo {
	return &ProjectRepo{q: q}
}

// Save persists a new project. UPDATE is not yet supported — the
// only mutation we need pre-MVP is create-on-bootstrap.
func (r *ProjectRepo) Save(ctx context.Context, p project.Project) error {
	tenantID := toPgUUID(p.TenantID)
	_, err := r.q.CreateProject(ctx, db.CreateProjectParams{
		TenantID:      tenantID,
		Slug:          p.Slug.String(),
		Name:          p.Name.String(),
		DefaultLocale: p.DefaultLocale,
		ApiKeyHash:    p.APIKeyHash,
	})
	return err
}

// Find loads a project by (tenantID, slug).
func (r *ProjectRepo) Find(ctx context.Context, tenantID uuid.UUID, slug project.Slug) (project.Project, error) {
	row, err := r.q.GetProjectBySlug(ctx, db.GetProjectBySlugParams{
		TenantID: toPgUUID(tenantID),
		Slug:     slug.String(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return project.Project{}, fmt.Errorf("project %q: %w", slug, ErrNotFound)
		}
		return project.Project{}, err
	}
	return fromGetProjectBySlugRow(row)
}

// FindByAPIKeyHash powers the API-key auth middleware. Comparison is
// timing-safe at the driver level: Postgres byte equality on a
// fixed-length BYTEA.
func (r *ProjectRepo) FindByAPIKeyHash(ctx context.Context, hash []byte) (project.Project, error) {
	row, err := r.q.GetProjectByAPIKeyHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return project.Project{}, ErrNotFound
		}
		return project.Project{}, err
	}
	return fromGetProjectByAPIKeyHashRow(row)
}

// ListForTenant returns projects for the given tenant, newest first.
func (r *ProjectRepo) ListForTenant(ctx context.Context, tenantID uuid.UUID) ([]project.Project, error) {
	rows, err := r.q.ListProjectsForTenant(ctx, toPgUUID(tenantID))
	if err != nil {
		return nil, err
	}
	out := make([]project.Project, 0, len(rows))
	for _, row := range rows {
		out = append(out, project.Project{
			ID:            fromPgUUID(row.ID),
			TenantID:      tenantID,
			Slug:          project.Slug(row.Slug),
			Name:          project.Name(row.Name),
			DefaultLocale: row.DefaultLocale,
		})
	}
	return out, nil
}

func fromGetProjectBySlugRow(row db.Project) (project.Project, error) {
	return project.Project{
		ID:            fromPgUUID(row.ID),
		TenantID:      fromPgUUID(row.TenantID),
		Slug:          project.Slug(row.Slug),
		Name:          project.Name(row.Name),
		DefaultLocale: row.DefaultLocale,
		APIKeyHash:    row.ApiKeyHash,
	}, nil
}

func fromGetProjectByAPIKeyHashRow(row db.GetProjectByAPIKeyHashRow) (project.Project, error) {
	return project.Project{
		ID:            fromPgUUID(row.ID),
		TenantID:      fromPgUUID(row.TenantID),
		Slug:          project.Slug(row.Slug),
		Name:          project.Name(row.Name),
		DefaultLocale: row.DefaultLocale,
	}, nil
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func fromPgUUID(v pgtype.UUID) uuid.UUID {
	return uuid.UUID(v.Bytes)
}
