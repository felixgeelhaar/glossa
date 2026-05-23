package sqlcadapter

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/felixgeelhaar/glossa/apps/api/internal/db"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/translation"
)

// TranslationRepo is the sqlc-backed Repository for translations.
type TranslationRepo struct {
	q *db.Queries
}

// NewTranslationRepo wires the repo.
func NewTranslationRepo(q *db.Queries) *TranslationRepo {
	return &TranslationRepo{q: q}
}

// Upsert performs the (key, locale) merge on the translations table.
func (r *TranslationRepo) Upsert(ctx context.Context, t translation.Translation) (translation.Translation, error) {
	row, err := r.q.UpsertTranslation(ctx, db.UpsertTranslationParams{
		KeyID:     toPgUUID(t.KeyID),
		LocaleID:  toPgUUID(t.LocaleID),
		Value:     t.Value,
		Status:    string(t.Status),
		UpdatedBy: nullableUUID(t.UpdatedBy),
	})
	if err != nil {
		return translation.Translation{}, err
	}
	out := translation.Translation{
		ID:        fromPgUUID(row.ID),
		KeyID:     fromPgUUID(row.KeyID),
		LocaleID:  fromPgUUID(row.LocaleID),
		Value:     row.Value,
		Status:    translation.Status(row.Status),
		UpdatedAt: row.UpdatedAt.Time,
	}
	if row.UpdatedBy.Valid {
		out.UpdatedBy = uuid.UUID(row.UpdatedBy.Bytes)
	}
	return out, nil
}

// ListBundle returns every key for a project paired with its (maybe
// empty) translation in the requested locale.
func (r *TranslationRepo) ListBundle(ctx context.Context, projectID, localeID uuid.UUID) ([]translation.BundleEntry, error) {
	rows, err := r.q.ListBundle(ctx, db.ListBundleParams{
		ProjectID: toPgUUID(projectID),
		LocaleID:  toPgUUID(localeID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]translation.BundleEntry, 0, len(rows))
	for _, row := range rows {
		entry := translation.BundleEntry{Key: row.Key}
		if row.Value != nil {
			entry.Value = *row.Value
		}
		if row.Status != nil {
			entry.Status = translation.Status(*row.Status)
		}
		out = append(out, entry)
	}
	return out, nil
}

// nullableUUID wraps an optional UUID for sqlc's pgtype-based nullable
// shape. uuid.Nil maps to Valid=false so the column lands as NULL —
// useful for system-generated translation rows (CLI seed drops,
// machine-translation passthroughs) that have no human author.
func nullableUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{Valid: false}
	}
	return toPgUUID(id)
}
