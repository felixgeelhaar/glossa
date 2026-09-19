package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// principalSweeper is the background principal of the proposal sweep:
// it obsoletes proposed messages and nothing else. Its name is stable —
// it appears as the actor in events and audits.
const principalSweeper = "catalog.proposal_sweep"

// sweeperBatch bounds the tenants one pass takes.
const sweeperBatch = 50

// ProposalSweeper obsoletes the proposed messages of branches that
// closed (or merged without the default branch bringing them in) at
// least domain.ProposalRetention ago, across tenants, each in its
// tenant's scope (RFC 0004 §4.1). A reopened branch, or a later push of
// the key, proposes them again with their translations and history.
type ProposalSweeper struct {
	s        *Service
	scanner  Scanner
	interval time.Duration
	logger   *slog.Logger
}

// NewProposalSweeper returns a sweeper that looks for expired proposals
// every interval (default an hour).
func NewProposalSweeper(s *Service, scanner Scanner, interval time.Duration, logger *slog.Logger) *ProposalSweeper {
	if interval <= 0 {
		interval = time.Hour
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &ProposalSweeper{s: s, scanner: scanner, interval: interval, logger: logger}
}

// Run sweeps every interval until ctx is cancelled.
func (p *ProposalSweeper) Run(ctx context.Context) error {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		if n, err := p.RunOnce(ctx); err != nil && ctx.Err() == nil {
			p.logger.ErrorContext(ctx, "catalog: sweeping expired proposals", slog.Any("error", err))
		} else if n > 0 {
			p.logger.InfoContext(ctx, "catalog: proposed messages obsoleted", slog.Int("messages", n))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// RunOnce sweeps the tenants with expired proposals and returns how
// many messages it obsoleted. One failing tenant doesn't hold up the
// others; their errors are joined.
func (p *ProposalSweeper) RunOnce(ctx context.Context) (int, error) {
	cutoff := p.s.now().Add(-domain.ProposalRetention)
	tenants, err := p.scanner.TenantsWithExpiredProposals(ctx, cutoff, sweeperBatch)
	if err != nil {
		return 0, err
	}
	var (
		n    int
		errs []error
	)
	for _, t := range tenants {
		bg, err := authz.Background(tenancy.ContextWithTenant(ctx, t), principalSweeper, authz.CatalogWrite)
		if err != nil {
			return n, err
		}
		swept, err := p.s.SweepProposals(bg)
		if err != nil {
			p.logger.ErrorContext(ctx, "catalog: proposal sweep", slog.String("tenant_id", t.String()), slog.Any("error", err))
			errs = append(errs, err)
			continue
		}
		n += swept
	}
	return n, errors.Join(errs...)
}
