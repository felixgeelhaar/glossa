package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/intelligence/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
)

// Fill limits.
const (
	MaxFillLocales = 20
	MaxFillKeys    = 500
	// MaxFillJobs bounds one fill; a larger catalog is filled in several
	// requests (filters) or through auto-translate.
	MaxFillJobs = 20_000
	fillPage    = 100
)

// Warnings a fill answers with when jobs will fail or do little.
const (
	WarnProviderConsentOff = "provider_consent_off"
	WarnNoBudget           = "no_budget"
	WarnNoProvider         = "no_provider"
)

// FillRequest asks to translate a project's messages into locales.
type FillRequest struct {
	Locales []string
	Filter  FillFilter
}

// FillResult is a created (or replayed) fill with its job counts and
// warnings.
type FillResult struct {
	Fill     Fill
	Counts   map[domain.JobState]int
	Warnings []string
}

// RequestFill queues jobs for the project's messages missing (or, with
// include_outdated, outdated) in each locale — Studio's "Fill with AI"
// and the CLI's translate. Sensitive namespaces are skipped. Jobs that
// exist for the same message, locale, source revision and knowledge are
// reused (failed, dead and cancelled ones are queued again). Needs
// intelligence.translate for every locale.
func (s *Service) RequestFill(ctx context.Context, project uuid.UUID, req FillRequest, idemKey string) (FillResult, bool, error) {
	if len(req.Locales) == 0 || len(req.Locales) > MaxFillLocales {
		return FillResult{}, false, ErrTooManyLocales
	}
	if len(req.Filter.Keys) > MaxFillKeys {
		return FillResult{}, false, ErrTooManyKeys
	}
	locales, err := canonicalLocales(req.Locales)
	if err != nil {
		return FillResult{}, false, err
	}
	var by string
	for _, l := range locales {
		if by, err = actorFor(ctx, authz.IntelligenceTranslate, l); err != nil {
			return FillResult{}, false, err
		}
	}
	id, err := idempotentID(ctx, "intelligence.fill", by, idemKey)
	if err != nil {
		return FillResult{}, false, err
	}
	if prior, found, err := s.existingFill(ctx, id); err != nil || found {
		if err == nil && (prior.ProjectID != project || !slices.Equal(prior.Locales, locales)) {
			err = ErrIdempotencyReuse
		}
		res, werr := s.fillResult(ctx, prior)
		return res, found, errors.Join(err, werr)
	}
	if _, err := s.Catalog.Project(ctx, project); err != nil {
		return FillResult{}, false, err
	}
	have, err := s.Localization.Locales(ctx, project)
	if err != nil {
		return FillResult{}, false, err
	}
	for _, l := range locales {
		if !slices.Contains(have, l) {
			return FillResult{}, false, fmt.Errorf("%w: %s", ErrLocaleNotFound, l)
		}
	}
	f := Fill{ID: id, ProjectID: project, Trigger: domain.TriggerFill, Locales: locales, Filter: req.Filter, RequestedBy: by, CreatedAt: s.Now()}
	if err := s.fill(ctx, &f, true); err != nil {
		return FillResult{}, false, err
	}
	res, err := s.fillResult(ctx, f)
	return res, false, err
}

func (s *Service) existingFill(ctx context.Context, id uuid.UUID) (Fill, bool, error) {
	var f Fill
	err := s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		f, err = st.Fill(ctx, id)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return Fill{}, false, nil
	}
	return f, err == nil, err
}

// fillResult adds the job counts and warnings.
func (s *Service) fillResult(ctx context.Context, f Fill) (FillResult, error) {
	res := FillResult{Fill: f}
	err := s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		if f.ID != uuid.Nil {
			if res.Counts, err = st.FillJobCounts(ctx, f.ID); err != nil {
				return err
			}
		}
		settings, err := s.settings(ctx, st, false)
		if err != nil {
			return err
		}
		providers, err := st.AllProviders(ctx)
		if err != nil {
			return err
		}
		if !settings.ProviderConsent {
			res.Warnings = append(res.Warnings, WarnProviderConsentOff)
		}
		if settings.MonthlyBudget == 0 {
			res.Warnings = append(res.Warnings, WarnNoBudget)
		}
		if !slices.ContainsFunc(providers, func(p StoredProvider) bool { return p.Enabled }) {
			res.Warnings = append(res.Warnings, WarnNoProvider)
		}
		return nil
	})
	return res, err
}

// GetFill returns a fill with its jobs' states. Needs intelligence.read.
func (s *Service) GetFill(ctx context.Context, id uuid.UUID) (FillResult, error) {
	if err := authz.Require(ctx, authz.IntelligenceRead); err != nil {
		return FillResult{}, err
	}
	f, found, err := s.existingFill(ctx, id)
	if err != nil {
		return FillResult{}, err
	}
	if !found {
		return FillResult{}, ErrNotFound
	}
	return s.fillResult(ctx, f)
}

// CancelFill cancels a fill's queued jobs; running ones finish. Needs
// intelligence.translate for the fill's locales.
func (s *Service) CancelFill(ctx context.Context, id uuid.UUID) (FillResult, error) {
	if err := authz.Require(ctx, authz.IntelligenceRead); err != nil {
		return FillResult{}, err
	}
	f, found, err := s.existingFill(ctx, id)
	if err != nil || !found {
		return FillResult{}, errors.Join(err, notFoundUnless(found))
	}
	var by string
	for _, l := range f.Locales {
		if by, err = actorFor(ctx, authz.IntelligenceTranslate, l); err != nil {
			return FillResult{}, err
		}
	}
	err = s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		_, err := st.CancelFillJobs(ctx, id, by, s.Now())
		return err
	})
	if err != nil {
		return FillResult{}, err
	}
	return s.fillResult(ctx, f)
}

func notFoundUnless(found bool) error {
	if found {
		return nil
	}
	return ErrNotFound
}

// fill enumerates the messages to translate per locale and queues a job
// for each. ctx carries a principal that may read the catalog and
// translations (the caller, or a background principal).
func (s *Service) fill(ctx context.Context, f *Fill, requeue bool) error {
	f.Skipped = map[string]int{}
	if err := s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error { return st.InsertFill(ctx, *f) }); err != nil {
		return err
	}
	ps, err := s.projectSettings(ctx, f.ProjectID)
	if err != nil {
		return err
	}
	q := newQueuer(s, f.ProjectID)
	total := 0
	for _, locale := range f.Locales {
		err := s.eachFillMessage(ctx, *f, locale, func(msgs []SourceMessage) error {
			var jobs []domain.Job
			for _, m := range msgs {
				if slices.Contains(ps.NamespaceTags.Of(m.Namespace), domain.TagSensitive) {
					f.Skipped[domain.FailureSensitive]++
					continue
				}
				if total >= MaxFillJobs {
					f.Skipped["limit"]++
					continue
				}
				total++
				j, err := q.job(ctx, m, locale, domain.TriggerFill, &f.ID, f.RequestedBy)
				if err != nil {
					return err
				}
				jobs = append(jobs, j)
			}
			created, existing, err := s.enqueue(ctx, jobs, requeue)
			f.JobsCreated += created
			f.JobsExisting += existing
			return err
		})
		if err != nil {
			return err
		}
	}
	return s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error { return st.FinishFill(ctx, *f) })
}

// eachFillMessage calls fn with pages of the messages a fill covers in
// locale: the listed keys that are missing or outdated there, or every
// active message missing there (and outdated, when asked), filtered.
func (s *Service) eachFillMessage(ctx context.Context, f Fill, locale string, fn func([]SourceMessage) error) error {
	if len(f.Filter.Keys) > 0 {
		msgs, err := s.Catalog.MessagesByKeys(ctx, f.ProjectID, f.Filter.Keys)
		if err != nil {
			return err
		}
		var todo []SourceMessage
		for _, m := range msgs {
			if !m.Active {
				continue
			}
			tr, err := s.Localization.Translation(ctx, f.ProjectID, m.Key, locale)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
			if tr.upToDate(m.Revision) && !(f.Filter.IncludeOutdated && tr.SourceRevision < m.Revision) {
				f.Skipped[domain.SkipUpToDate]++
				continue
			}
			todo = append(todo, m)
		}
		return fn(todo)
	}
	queries := []MessageQuery{{Namespace: f.Filter.Namespace, KeyPrefix: f.Filter.KeyPrefix, MissingIn: locale}}
	if f.Filter.IncludeOutdated {
		queries = append(queries, MessageQuery{Namespace: f.Filter.Namespace, KeyPrefix: f.Filter.KeyPrefix, OutdatedIn: locale})
	}
	for _, q := range queries {
		after := ""
		for {
			msgs, err := s.Catalog.Messages(ctx, f.ProjectID, q, after, fillPage)
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				break
			}
			if err := fn(msgs); err != nil {
				return err
			}
			if len(msgs) < fillPage {
				break
			}
			after = msgs[len(msgs)-1].Key
		}
	}
	return nil
}

// upToDate reports a usable translation made against the current
// source revision: nothing to translate.
func (t TranslationState) upToDate(revision int) bool {
	return t.Exists && t.State != "rejected" && t.SourceRevision >= revision
}

// enqueue stores jobs in one transaction and counts new and existing.
func (s *Service) enqueue(ctx context.Context, jobs []domain.Job, requeue bool) (created, existing int, err error) {
	if len(jobs) == 0 {
		return 0, 0, nil
	}
	err = s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		created, existing = 0, 0
		for _, j := range jobs {
			_, isNew, err := st.EnqueueJob(ctx, j, requeue)
			if err != nil {
				return err
			}
			if isNew {
				created++
			} else {
				existing++
			}
		}
		return nil
	})
	return created, existing, err
}

// queuer builds jobs with their knowledge fingerprints, caching the
// style sources and termbase versions per locale and namespace.
type queuer struct {
	s        *Service
	project  uuid.UUID
	source   string
	styles   map[string][]string
	termbase map[string]string
}

func newQueuer(s *Service, project uuid.UUID) *queuer {
	return &queuer{s: s, project: project, styles: map[string][]string{}, termbase: map[string]string{}}
}

func (q *queuer) job(ctx context.Context, m SourceMessage, locale string, trigger domain.Trigger, fill *uuid.UUID, by string) (domain.Job, error) {
	scope := domain.Scope{TenantID: tenantOf(ctx).String(), ProjectID: q.project.String()}
	if q.source == "" {
		p, err := q.s.Catalog.Project(ctx, q.project)
		if err != nil {
			return domain.Job{}, err
		}
		q.source = p.SourceLocale
	}
	styleKey := locale + "\n" + m.Namespace
	styles, ok := q.styles[styleKey]
	if !ok {
		var err error
		if styles, err = q.s.Knowledge.StyleSources(ctx, scope, locale, m.Namespace); err != nil {
			return domain.Job{}, err
		}
		q.styles[styleKey] = styles
	}
	tb, ok := q.termbase[locale]
	if !ok {
		var err error
		if tb, err = q.s.Knowledge.TermbaseVersion(ctx, scope, domain.LocalePair{Source: q.source, Target: locale}); err != nil {
			return domain.Job{}, err
		}
		q.termbase[locale] = tb
	}
	p := q.s.prompts
	fp := domain.Fingerprint{Prompts: strings.Join([]string{p.Translate, p.Repair, p.Assess}, ","), Styles: styles, Concepts: []string{tb}}
	now := q.s.Now()
	return domain.Job{
		ID: uuid.Must(uuid.NewV7()), ProjectID: q.project, MessageID: m.ID, MessageKey: m.Key, Namespace: m.Namespace,
		Locale: locale, SourceRevision: m.Revision, Fingerprint: fp.Sum(), Trigger: trigger, FillID: fill,
		State: domain.JobQueued, MaxAttempts: domain.DefaultMaxAttempts, AvailableAt: now, CreatedBy: by,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// ListJobs lists jobs, newest first. Needs intelligence.read.
func (s *Service) ListJobs(ctx context.Context, f JobFilter, page pagination.Page) ([]domain.Job, *string, error) {
	if err := authz.Require(ctx, authz.IntelligenceRead); err != nil {
		return nil, nil, err
	}
	if f.Locale != "" {
		t, err := bcp47.Parse(f.Locale)
		if err != nil {
			return nil, nil, err
		}
		f.Locale = t.String()
	}
	before, err := parseCursor(page.After)
	if err != nil {
		return nil, nil, err
	}
	var rows []domain.Job
	err = s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		rows, err = st.Jobs(ctx, f, before, page.Limit())
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	items, next := pagination.Trim(rows, page, func(j domain.Job) string { return Cursor{At: j.CreatedAt, ID: j.ID}.String() })
	return items, next, nil
}

// GetJob returns a job with its audit ledger. Needs intelligence.read.
func (s *Service) GetJob(ctx context.Context, id uuid.UUID) (JobView, error) {
	if err := authz.Require(ctx, authz.IntelligenceRead); err != nil {
		return JobView{}, err
	}
	var v JobView
	err := s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		v, err = st.Job(ctx, id)
		return err
	})
	return v, err
}

// CancelJob cancels a queued job. Needs intelligence.translate for its
// locale.
func (s *Service) CancelJob(ctx context.Context, id uuid.UUID) (JobView, error) {
	v, err := s.GetJob(ctx, id)
	if err != nil {
		return JobView{}, err
	}
	by, err := actorFor(ctx, authz.IntelligenceTranslate, v.Locale)
	if err != nil {
		return JobView{}, err
	}
	err = s.Tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		ok, err := st.CancelJob(ctx, id, by, s.Now())
		if err != nil {
			return err
		}
		if !ok {
			cur, err := st.Job(ctx, id)
			if err == nil && cur.State != domain.JobCancelled {
				return domain.ErrJobNotCancellable
			}
			return err
		}
		v, err = st.Job(ctx, id)
		return err
	})
	return v, err
}

// sinceDefault is the metrics window when none is given.
func sinceDefault(now time.Time) time.Time { return now.AddDate(0, 0, -30) }
