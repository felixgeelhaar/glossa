// Package coverage adapts Localization's application service to
// Catalog's TranslationCoverage and TranslationImpact ports, so the
// message list can filter by "missing in" and "outdated in" a locale,
// and a branch's status can count the translations it would make
// outdated, without Catalog reading Localization's tables.
package coverage

import (
	"context"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	"github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	locapp "github.com/felixgeelhaar/glossa/platform/internal/localization/app"
)

// Port implements app.TranslationCoverage.
type Port struct{ svc *locapp.Service }

// New returns the port.
func New(svc *locapp.Service) *Port { return &Port{svc: svc} }

var (
	_ app.TranslationCoverage = (*Port)(nil)
	_ app.TranslationImpact   = (*Port)(nil)
)

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
