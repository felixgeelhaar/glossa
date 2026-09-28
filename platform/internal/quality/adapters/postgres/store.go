// Package postgres implements Quality's persistence port on the
// kernel's unit of work, with sqlc queries over the quality_* tables of
// migration 0027. It is read-only for now: M4 wave 2 needs the stored
// findings on the read surfaces (the API and MCP), and the layers still
// write their runs through the slice that stores them.
package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/adapters/postgres/qualitysql"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// Transactor implements app.Transactor.
type Transactor struct{ uow *db.UnitOfWork }

// NewTransactor returns a Transactor on uow.
func NewTransactor(uow *db.UnitOfWork) *Transactor { return &Transactor{uow: uow} }

// InTenant implements app.Transactor.
func (t *Transactor) InTenant(ctx context.Context, fn func(context.Context, app.Store) error) error {
	return t.uow.InTenantTx(ctx, func(ctx context.Context, tx *db.TenantTx) error {
		return fn(ctx, &store{q: qualitysql.New(tx)})
	})
}

type store struct{ q *qualitysql.Queries }

var _ app.Store = (*store)(nil)

// LatestRun implements app.Store.
func (s *store) LatestRun(ctx context.Context, project uuid.UUID, ref string, completedOnly bool) (app.RunSummary, error) {
	row, err := s.q.LatestRun(ctx, qualitysql.LatestRunParams{
		ProjectID: project, Ref: ref, CompletedOnly: completedOnly,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.RunSummary{}, app.ErrNotFound
	}
	if err != nil {
		return app.RunSummary{}, err
	}
	run := app.RunSummary{
		ID: row.ID, Ref: row.Ref, Trigger: domain.Trigger(row.Trigger), PolicyVersion: int(row.PolicyVersion),
		Counts: domain.Counts{
			Errors: int(row.Errors), Warnings: int(row.Warnings), Waived: int(row.Waived),
		},
		Conclusion: domain.Conclusion(row.Conclusion.String), CreatedBy: row.CreatedBy, StartedAt: row.StartedAt,
	}
	for _, l := range row.Layers {
		run.Layers = append(run.Layers, domain.Layer(l))
	}
	if row.CompletedAt.Valid {
		at := row.CompletedAt.Time
		run.CompletedAt = &at
	}
	return run, nil
}

// Findings implements app.Store. An `after` that is not a UUID is read
// as the first page: a cursor this store never issued selects nothing
// rather than everything.
func (s *store) Findings(
	ctx context.Context, run uuid.UUID, f app.FindingFilter, after string, limit int,
) ([]app.StoredFinding, error) {
	cursor, err := uuid.Parse(after)
	if after != "" && err != nil {
		return nil, app.ErrInvalidQuery
	}
	rows, err := s.q.RunFindings(ctx, qualitysql.RunFindingsParams{
		RunID: run, Layer: string(f.Layer), Locale: f.Locale, Severity: string(f.Severity),
		MessageKey: f.MessageKey, WaivedOnly: f.WaivedOnly, After: cursor, RowLimit: int32(limit), //nolint:gosec // bounded by app.MaxFindingLimit
	})
	if err != nil {
		return nil, err
	}
	out := make([]app.StoredFinding, len(rows))
	for i, r := range rows {
		out[i] = app.StoredFinding{ID: r.ID, Finding: findingOf(r)}
	}
	return out, nil
}

// findingOf rebuilds the domain finding from its row. The stored
// fingerprint is kept rather than recomputed: it is what a waiver
// names, and a read must not quietly disagree with the run that wrote
// it.
func findingOf(r qualitysql.RunFindingsRow) domain.Finding {
	f := domain.Finding{
		Schema: domain.Schema, Fingerprint: r.Fingerprint, Layer: domain.Layer(r.Layer), Code: r.Code,
		Severity: domain.Severity(r.Severity), Message: r.Explanation, Subject: r.Subject, Detail: r.Detail,
		Locus: domain.Locus{
			Key: r.MessageKey, Locale: r.Locale, Namespace: r.Namespace, File: r.File,
			Line: int(r.Line.Int32), Column: int(r.Col.Int32), Route: r.Route, Component: r.Component,
			Region: r.Region,
		},
	}
	if r.MessageID.Valid {
		f.Locus.Message = r.MessageID.UUID.String()
	}
	if r.CaptureID.Valid {
		f.Locus.Capture = r.CaptureID.UUID.String()
	}
	if r.SpanSide.Valid {
		f.Locus.Span = &domain.Span{
			Side: domain.Side(r.SpanSide.String), Start: int(r.SpanStart.Int32), End: int(r.SpanEnd.Int32),
		}
	}
	if r.SourceRevision.Valid {
		rev := int(r.SourceRevision.Int32)
		f.SourceRevision = &rev
	}
	if r.WaiverID.Valid {
		f.Waiver = r.WaiverID.UUID.String()
	}
	return f
}
