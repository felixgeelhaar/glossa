package remote

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"

	"github.com/felixgeelhaar/glossa/platform/internal/apiclient"
)

// Types the CLI reads, re-exported so commands don't import the
// generated package.
type (
	Tenant                = apiclient.Tenant
	Project               = apiclient.Project
	ProjectLocale         = apiclient.ProjectLocale
	Message               = apiclient.Message
	Translation           = apiclient.Translation
	QAFinding             = apiclient.QAFinding
	MessageUpsertItem     = apiclient.MessageUpsertItem
	MessageUpsertResult   = apiclient.MessageUpsertItemResult
	TranslationImportItem = apiclient.TranslationImportItem
	TranslationImportRes  = apiclient.TranslationImportItemResult
	ReviewState           = apiclient.ReviewState
	Syntax                = apiclient.Syntax
)

// Scope is the tenant and project a request acts in.
type Scope struct {
	Tenant  string
	Project string
}

func (c *Client) path(format string, args ...any) string {
	esc := make([]any, len(args))
	for i, a := range args {
		esc[i] = url.PathEscape(fmt.Sprint(a))
	}
	return c.server + fmt.Sprintf(format, esc...)
}

// collect pages through a list operation.
func collect[T any](fetch func(token *string) ([]T, *string, error)) ([]T, error) {
	var (
		all   []T
		token *string
	)
	for {
		items, next, err := fetch(token)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if next == nil || *next == "" {
			return all, nil
		}
		token = next
	}
}

// Tenants lists the tenants the caller can act in; for an API token, its
// own tenant.
func (c *Client) Tenants(ctx context.Context) ([]Tenant, error) {
	size := pageSize
	return collect(func(tok *string) ([]Tenant, *string, error) {
		r, err := c.api.ListTenantsWithResponse(ctx, &apiclient.ListTenantsParams{PageSize: &size, PageToken: tok})
		if err := check(r, err, http.MethodGet, c.path("/v1/tenants")); err != nil {
			return nil, nil, err
		}
		return r.JSON200.Items, r.JSON200.NextPageToken, nil
	})
}

// Projects lists a tenant's projects.
func (c *Client) Projects(ctx context.Context, tenant string) ([]Project, error) {
	size := pageSize
	return collect(func(tok *string) ([]Project, *string, error) {
		r, err := c.api.ListProjectsWithResponse(ctx, tenant, &apiclient.ListProjectsParams{PageSize: &size, PageToken: tok})
		if err := check(r, err, http.MethodGet, c.path("/v1/tenants/%s/projects", tenant)); err != nil {
			return nil, nil, err
		}
		return r.JSON200.Items, r.JSON200.NextPageToken, nil
	})
}

// ErrProjectNotFound means no project has the slug or ID.
type ErrProjectNotFound struct{ Ref, Tenant string }

func (e *ErrProjectNotFound) Error() string {
	return fmt.Sprintf("no project %q in tenant %s", e.Ref, e.Tenant)
}

// ResolveProject finds a project by slug or ID.
func (c *Client) ResolveProject(ctx context.Context, tenant, ref string) (Project, error) {
	projects, err := c.Projects(ctx, tenant)
	if err != nil {
		return Project{}, err
	}
	for _, p := range projects {
		if p.Id == ref || string(p.Slug) == ref {
			return p, nil
		}
	}
	return Project{}, &ErrProjectNotFound{Ref: ref, Tenant: tenant}
}

// Locales lists a project's locales, the source locale included.
func (c *Client) Locales(ctx context.Context, s Scope) ([]ProjectLocale, error) {
	size := pageSize
	return collect(func(tok *string) ([]ProjectLocale, *string, error) {
		r, err := c.api.ListLocalesWithResponse(ctx, s.Tenant, s.Project, &apiclient.ListLocalesParams{PageSize: &size, PageToken: tok})
		if err := check(r, err, http.MethodGet, c.path("/v1/tenants/%s/projects/%s/locales", s.Tenant, s.Project)); err != nil {
			return nil, nil, err
		}
		return r.JSON200.Items, r.JSON200.NextPageToken, nil
	})
}

// AddLocale adds a locale (idempotently); created is false when the
// project already had it.
func (c *Client) AddLocale(ctx context.Context, s Scope, code string) (ProjectLocale, bool, error) {
	r, err := c.api.AddLocaleWithResponse(ctx, s.Tenant, s.Project, apiclient.AddLocale{Code: code})
	if err := check(r, err, http.MethodPost, c.path("/v1/tenants/%s/projects/%s/locales", s.Tenant, s.Project)); err != nil {
		return ProjectLocale{}, false, err
	}
	if r.JSON201 != nil {
		return *r.JSON201, true, nil
	}
	return *r.JSON200, false, nil
}

// FallbackGraph returns the project's fallback graph.
func (c *Client) FallbackGraph(ctx context.Context, s Scope) (map[string][]string, error) {
	r, err := c.api.GetFallbackGraphWithResponse(ctx, s.Tenant, s.Project)
	if err := check(r, err, http.MethodGet, c.path("/v1/tenants/%s/projects/%s/fallback-graph", s.Tenant, s.Project)); err != nil {
		return nil, err
	}
	return r.JSON200.Fallback, nil
}

// MessageFilter narrows Messages.
type MessageFilter struct {
	State      string
	KeyPrefix  string
	Namespace  string
	MissingIn  string
	OutdatedIn string
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Messages lists a project's messages by key.
func (c *Client) Messages(ctx context.Context, s Scope, f MessageFilter) ([]Message, error) {
	size := pageSize
	params := apiclient.ListMessagesParams{
		PageSize: &size, KeyPrefix: optional(f.KeyPrefix), Namespace: optional(f.Namespace),
		MissingIn: optional(f.MissingIn), OutdatedIn: optional(f.OutdatedIn),
	}
	if f.State != "" {
		st := apiclient.MessageState(f.State)
		params.State = &st
	}
	return collect(func(tok *string) ([]Message, *string, error) {
		p := params
		p.PageToken = tok
		r, err := c.api.ListMessagesWithResponse(ctx, s.Tenant, s.Project, &p)
		if err := check(r, err, http.MethodGet, c.path("/v1/tenants/%s/projects/%s/messages", s.Tenant, s.Project)); err != nil {
			return nil, nil, err
		}
		return r.JSON200.Items, r.JSON200.NextPageToken, nil
	})
}

// MessageTranslations lists one message's translations.
func (c *Client) MessageTranslations(ctx context.Context, s Scope, key string) ([]Translation, error) {
	size := pageSize
	return collect(func(tok *string) ([]Translation, *string, error) {
		r, err := c.api.ListMessageTranslationsWithResponse(ctx, s.Tenant, s.Project, key,
			&apiclient.ListMessageTranslationsParams{PageSize: &size, PageToken: tok})
		if err := check(r, err, http.MethodGet,
			c.path("/v1/tenants/%s/projects/%s/messages/%s/translations", s.Tenant, s.Project, key)); err != nil {
			return nil, nil, err
		}
		return r.JSON200.Items, r.JSON200.NextPageToken, nil
	})
}

// AllTranslations reads the translations of every key, a few messages at
// a time. The /v1 API has no project-wide translation list yet, so this
// is one request per message (bounded concurrency).
func (c *Client) AllTranslations(ctx context.Context, s Scope, keys []string) (map[string][]Translation, error) {
	const workers = 8
	out := make(map[string][]Translation, len(keys))
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	next := make(chan string)
	for range workers {
		wg.Go(func() {
			for key := range next {
				trs, err := c.MessageTranslations(ctx, s, key)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
					cancel()
				}
				out[key] = trs
				mu.Unlock()
			}
		})
	}
	for _, k := range keys {
		select {
		case next <- k:
		case <-ctx.Done():
		}
	}
	close(next)
	wg.Wait()
	return out, firstErr
}

// UpsertMessages creates or revises messages in batches of MaxBatch.
// Results come back in item order.
func (c *Client) UpsertMessages(ctx context.Context, s Scope, items []MessageUpsertItem) ([]MessageUpsertResult, error) {
	var out []MessageUpsertResult
	for start := 0; start < len(items); start += MaxBatch {
		batch := items[start:min(start+MaxBatch, len(items))]
		r, err := c.api.UpsertMessagesWithResponse(idempotent(ctx), s.Tenant, s.Project, apiclient.MessageUpsert{Items: batch})
		if err := check(r, err, http.MethodPost, c.path("/v1/tenants/%s/projects/%s/message-upserts", s.Tenant, s.Project)); err != nil {
			return out, err
		}
		out = append(out, r.JSON200.Results...)
	}
	return out, nil
}

// ImportTranslations writes translations with provenance import, in
// batches of MaxBatch. Results come back in item order.
func (c *Client) ImportTranslations(ctx context.Context, s Scope, items []TranslationImportItem) ([]TranslationImportRes, error) {
	var out []TranslationImportRes
	for start := 0; start < len(items); start += MaxBatch {
		batch := items[start:min(start+MaxBatch, len(items))]
		r, err := c.api.ImportTranslationsWithResponse(idempotent(ctx), s.Tenant, s.Project, apiclient.TranslationImport{Items: batch})
		if err := check(r, err, http.MethodPost, c.path("/v1/tenants/%s/projects/%s/translation-imports", s.Tenant, s.Project)); err != nil {
			return out, err
		}
		out = append(out, r.JSON200.Results...)
	}
	return out, nil
}

// SortLocales puts the source locale first, then the rest by code.
func SortLocales(ls []ProjectLocale) {
	sort.SliceStable(ls, func(i, j int) bool {
		if ls[i].IsSource != ls[j].IsSource {
			return ls[i].IsSource
		}
		return ls[i].Code < ls[j].Code
	})
}
