package sqlcadapter

import (
	"context"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/apps/api/internal/db"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/locale"
)

// LocaleRepo is the sqlc-backed Repository for locales.
type LocaleRepo struct {
	q *db.Queries
}

// NewLocaleRepo wires the repo.
func NewLocaleRepo(q *db.Queries) *LocaleRepo {
	return &LocaleRepo{q: q}
}

// Save creates a new locale row.
func (r *LocaleRepo) Save(ctx context.Context, l locale.Locale) error {
	_, err := r.q.CreateLocale(ctx, db.CreateLocaleParams{
		ProjectID: toPgUUID(l.ProjectID),
		Code:      l.Code.String(),
		Label:     l.Label.String(),
		Enabled:   l.Enabled,
	})
	return err
}

// ListForProject returns every locale defined under a project.
func (r *LocaleRepo) ListForProject(ctx context.Context, projectID uuid.UUID) ([]locale.Locale, error) {
	rows, err := r.q.ListLocalesForProject(ctx, toPgUUID(projectID))
	if err != nil {
		return nil, err
	}
	out := make([]locale.Locale, 0, len(rows))
	for _, row := range rows {
		out = append(out, locale.Locale{
			ID:        fromPgUUID(row.ID),
			ProjectID: projectID,
			Code:      locale.Code(row.Code),
			Label:     locale.Label(row.Label),
			Enabled:   row.Enabled,
		})
	}
	return out, nil
}

// SetEnabled flips the enabled flag on a locale.
func (r *LocaleRepo) SetEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	return r.q.SetLocaleEnabled(ctx, db.SetLocaleEnabledParams{
		ID:      toPgUUID(id),
		Enabled: enabled,
	})
}
