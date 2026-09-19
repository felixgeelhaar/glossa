package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/intelligence/domain"
)

// jobRuntime is what one job runs with: the tenant's settings, the
// project's, and a router over the tenant's enabled providers.
type jobRuntime struct {
	settings domain.TenantSettings
	project  domain.ProjectSettings
	router   *Router
}

// runtimeFor loads a job's configuration and builds its router: the
// routing policy in effect (project → tenant → default), the effective
// prices and the tenant's budget, over the enabled providers (each
// limited to its model allow-list).
func (s *Service) runtimeFor(ctx context.Context, j domain.Job) (jobRuntime, error) {
	var (
		rt        jobRuntime
		providers []StoredProvider
		policy    RoutingView
	)
	err := s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		if rt.settings, err = s.settings(ctx, st, false); err != nil {
			return err
		}
		ps, found, err := st.ProjectSettings(ctx, j.ProjectID)
		if err != nil {
			return err
		}
		rt.project = domain.DefaultProjectSettings(j.ProjectID)
		if found {
			rt.project = ps
		}
		if providers, err = st.AllProviders(ctx); err != nil {
			return err
		}
		policy, err = routing(ctx, st, &j.ProjectID)
		return err
	})
	if err != nil {
		return jobRuntime{}, err
	}
	tenant := tenantOf(ctx)
	byName := map[string]domain.Provider{}
	for _, p := range providers {
		if !p.Enabled {
			continue
		}
		// A provider that can't be built (its key doesn't open, or is
		// missing) is left out: routes to it fail as unconfigured, and the
		// job says so, instead of retrying what waiting can't fix.
		key, err := s.openKey(tenant, p)
		if err != nil {
			s.Logger.WarnContext(ctx, "intelligence: provider unusable", slog.String("provider", p.Name), slog.Any("error", err))
			continue
		}
		prov, err := s.Providers.Provider(p.ProviderConfig, key)
		if err != nil {
			s.Logger.WarnContext(ctx, "intelligence: provider unusable", slog.String("provider", p.Name), slog.Any("error", err))
			continue
		}
		byName[p.Name] = allowListed{next: prov, cfg: p.ProviderConfig}
	}
	prices := s.effectivePrices(rt.settings, providers)
	budget := &budgetGuard{s: s, tenant: tenant, job: j.ID, project: j.ProjectID, prices: prices}
	rt.router = NewRouter(byName, policy.Record.Policy, prices, budget)
	return rt, nil
}

// allowListed refuses models outside a provider's allow-list, as a
// rejected request: the router falls back to the next route.
type allowListed struct {
	next domain.Provider
	cfg  domain.ProviderConfig
}

func (a allowListed) Name() string { return a.cfg.Name }

func (a allowListed) Complete(ctx context.Context, req domain.CompletionRequest) (domain.Completion, error) {
	if !a.cfg.Allows(req.Model) {
		return domain.Completion{}, &domain.ProviderError{
			Provider: a.cfg.Name, Kind: domain.KindInvalidRequest,
			Err: fmt.Errorf("model %q is not in the provider's allow-list", req.Model),
		}
	}
	return a.next.Complete(ctx, req)
}

// budgetGuard is the tenant's monthly budget (RFC 0003 §3.1): a call is
// refused when this month's spend plus its upper-bound estimate would
// pass the cap, and every priced call is booked in the ledger. A tenant
// without a budget (0) can't spend at all: the hard stop. Calls in
// flight when the cap is reached finish, so a month can end at most
// their cost over it.
type budgetGuard struct {
	s       *Service
	tenant  uuid.UUID
	job     uuid.UUID
	project uuid.UUID
	prices  domain.PriceTable
}

var _ domain.BudgetGuard = (*budgetGuard)(nil)

// Check implements domain.BudgetGuard.
func (b *budgetGuard) Check(ctx context.Context, _ domain.Scope, estimate domain.MicroUSD) error {
	return b.s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		settings, err := b.s.settings(ctx, st, false)
		if err != nil {
			return err
		}
		spent, _, err := st.SpendSince(ctx, monthStart(b.s.Now()))
		if err != nil {
			return err
		}
		if spent+estimate > settings.MonthlyBudget {
			return fmt.Errorf("%w: this month's spend is %v of the %v budget and the call may cost up to %v",
				domain.ErrBudgetExceeded, spent, settings.MonthlyBudget, estimate)
		}
		return nil
	})
}

// Record implements domain.BudgetGuard.
func (b *budgetGuard) Record(ctx context.Context, _ domain.Scope, sp domain.Spend) error {
	job, project := b.job, b.project
	_, priced := b.prices[domain.PriceKey(sp.Provider, sp.Model)]
	e := SpendEntry{ID: uuid.Must(uuid.NewV7()), JobID: &job, ProjectID: &project, Spend: sp, Priced: priced, OccurredAt: b.s.Now()}
	if err := b.s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error { return st.InsertSpend(ctx, e) }); err != nil {
		return err
	}
	b.s.Metrics.Spent(b.tenant, sp.Provider, sp.Model, sp.Cost)
	return nil
}

// jobKnowledge is the agent's Knowledge for one job: translation memory,
// termbase and style from the Knowledge context, and the message's
// context from Catalog and Localization.
type jobKnowledge struct {
	Knowledge
	s   *Service
	msg SourceMessage
}

var _ domain.Knowledge = (*jobKnowledge)(nil)

// neighbourLimit is how many neighbouring messages the context shows.
const neighbourLimit = 5

// MessageContext implements domain.MessageContexts: description, max
// length and neighbours by key prefix with their translations. The
// namespace's policy tags travel on the request (not here), so they are
// counted once.
func (k *jobKnowledge) MessageContext(ctx context.Context, _ domain.Scope, _ string, targetLocale string) (domain.MessageContext, error) {
	c := domain.MessageContext{
		MessageID: k.msg.ID.String(), Key: k.msg.Key, Namespace: k.msg.Namespace,
		Description: k.msg.Description, MaxLength: k.msg.MaxLength,
	}
	prefix, _, ok := cutLast(k.msg.Key, ".")
	if !ok {
		return c, nil
	}
	ns, err := k.s.Localization.Neighbours(ctx, k.msg.ProjectID, prefix+".", k.msg.Key, targetLocale, neighbourLimit)
	if err != nil {
		return domain.MessageContext{}, err
	}
	for _, n := range ns {
		c.Neighbours = append(c.Neighbours, domain.Neighbour{Key: n.Key, Source: n.SourceMF2, Translation: n.Translation})
	}
	return c, nil
}

func cutLast(s, sep string) (before, after string, found bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// sourceText is a message's canonical MF2 syntax.
func sourceText(m mf.Message) (string, error) {
	text, err := mf.Stringify(m)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrUnparsable, err)
	}
	return text, nil
}
