package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/postgres/catalogsql"
	"github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// Scanner implements app.Scanner in system scope: it reads only the
// columns migration 0016 opens to glossa_system (branch states, the
// proposal links and message states), and the sweep it schedules runs
// in each tenant's scope.
type Scanner struct {
	uow   *db.UnitOfWork
	sweep db.SystemScope
}

// NewScanner returns a Scanner on uow.
func NewScanner(uow *db.UnitOfWork) *Scanner {
	return &Scanner{uow: uow, sweep: db.NewSystemScope("catalog.proposal_sweep")}
}

var _ app.Scanner = (*Scanner)(nil)

// TenantsWithExpiredProposals implements app.Scanner.
func (s *Scanner) TenantsWithExpiredProposals(ctx context.Context, cutoff time.Time, limit int) ([]tenancy.ID, error) {
	var out []tenancy.ID
	err := s.uow.InSystemTx(ctx, s.sweep, func(ctx context.Context, tx *db.SystemTx) error {
		rows, err := catalogsql.New(tx).ListTenantsWithExpiredProposals(ctx, catalogsql.ListTenantsWithExpiredProposalsParams{
			Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true}, MaxRows: int32Of(limit),
		})
		for _, id := range rows {
			out = append(out, tenancy.ID(id))
		}
		return err
	})
	return out, err
}
