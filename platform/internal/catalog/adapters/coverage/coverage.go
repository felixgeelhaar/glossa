// Package coverage adapts Localization's application service to
// Catalog's TranslationCoverage, TranslationImpact and ProjectLocales
// ports, so the message list can filter by "missing in" and "outdated
// in" a locale, a branch's status can count the translations it would
// make outdated, and a check policy's required locales can be checked
// against the project's — all without Catalog reading Localization's
// tables.
package coverage

import (
	"context"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	"github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
	locapp "github.com/felixgeelhaar/glossa/platform/internal/localization/app"
)

// Port implements app.TranslationCoverage.
type Port struct{ svc *locapp.Service }

// New returns the port.
func New(svc *locapp.Service) *Port { return &Port{svc: svc} }

var (
	_ app.TranslationCoverage = (*Port)(nil)
	_ app.TranslationImpact   = (*Port)(nil)
	_ app.ProjectLocales      = (*Port)(nil)
)

// LocaleCodes implements app.ProjectLocales, paging through the
// project's locales. A project has a handful, so this walks them all.
func (p *Port) LocaleCodes(ctx context.Context, project domain.ProjectID) ([]string, error) {
	var out []string
	page := pagination.Page{Size: pagination.MaxPageSize}
	for {
		ls, next, err := p.svc.ListLocales(ctx, project.UUID(), page)
		if err != nil {
			return nil, err
		}
		for _, l := range ls {
			out = append(out, l.Code.String())
		}
		// The next_page_token is the HTTP cursor; in process the cursor
		// is the sort key itself, which for locales is the code.
		if next == nil || len(ls) == 0 {
			return out, nil
		}
		page.After = ls[len(ls)-1].Code.String()
	}
}

// CurrentTranslations implements app.TranslationImpact.
func (p *Port) CurrentTranslations(ctx context.Context, project domain.ProjectID, ids []domain.MessageID) (map[string]int, error) {
	uuids := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		uuids[i] = id.UUID()
	}
	return p.svc.CurrentTranslations(ctx, project.UUID(), uuids)
}

// MessagesWithCoverage implements app.TranslationCoverage.
func (p *Port) MessagesWithCoverage(ctx context.Context, q app.CoverageQuery) ([]domain.MessageID, error) {
	f := locapp.CoverageFilter{
		Locale: q.Locale, Outdated: q.Status == app.CoverageOutdated, KeyPrefix: q.Filter.KeyPrefix,
		AfterKey: q.AfterKey, Limit: q.Limit,
	}
	if q.Filter.Namespace != nil {
		ns := string(*q.Filter.Namespace)
		f.Namespace = &ns
	}
	if q.Filter.State != nil {
		st := string(*q.Filter.State)
		f.State = &st
	}
	ids, err := p.svc.MessagesWithCoverage(ctx, q.Project.UUID(), f)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MessageID, len(ids))
	for i, id := range ids {
		out[i] = domain.MessageID(id)
	}
	return out, nil
}
