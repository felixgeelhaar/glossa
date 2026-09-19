// Package sources adapts Catalog's and Localization's application
// services to Knowledge's ports: the only way Knowledge reads projects
// and translations (RFC 0002 §4, contexts reach each other through
// application ports).
package sources

import (
	"context"
	"errors"

	"github.com/google/uuid"

	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	catalogdomain "github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/knowledge/app"
	"github.com/felixgeelhaar/glossa/platform/internal/knowledge/domain"
	localizationapp "github.com/felixgeelhaar/glossa/platform/internal/localization/app"
	localizationdomain "github.com/felixgeelhaar/glossa/platform/internal/localization/domain"
)

// Projects implements app.Projects on Catalog's service.
type Projects struct{ svc *catalogapp.Service }

// NewProjects returns the port.
func NewProjects(svc *catalogapp.Service) *Projects { return &Projects{svc: svc} }

var _ app.Projects = (*Projects)(nil)

// Project implements app.Projects.
func (p *Projects) Project(ctx context.Context, id uuid.UUID) (app.ProjectInfo, error) {
	pr, err := p.svc.GetProject(ctx, catalogdomain.ProjectID(id))
	if errors.Is(err, catalogapp.ErrNotFound) {
		return app.ProjectInfo{}, app.ErrProjectNotFound
	}
	if err != nil {
		return app.ProjectInfo{}, err
	}
	return app.ProjectInfo{ID: id, SourceLocale: pr.SourceLocale}, nil
}

// Translations implements app.Translations on Localization's service.
type Translations struct{ svc *localizationapp.Service }

// NewTranslations returns the port.
func NewTranslations(svc *localizationapp.Service) *Translations { return &Translations{svc: svc} }

var _ app.Translations = (*Translations)(nil)

// Current implements app.Translations.
func (t *Translations) Current(ctx context.Context, project, translation uuid.UUID) (app.CurrentTranslation, error) {
	tr, err := t.svc.TranslationWithSource(ctx, project, localizationdomain.TranslationID(translation))
	if errors.Is(err, localizationapp.ErrNotFound) {
		return app.CurrentTranslation{}, app.ErrNotFound
	}
	if err != nil {
		return app.CurrentTranslation{}, err
	}
	return app.CurrentTranslation{
		ApprovedText: domain.ApprovedText{
			TranslationID: tr.ID.UUID(), ProjectID: tr.ProjectID, MessageID: tr.MessageID, Revision: tr.Revision,
			MessageKey: tr.Key, Namespace: tr.Namespace, SourceLocale: tr.SourceLocale, TargetLocale: tr.Locale,
			Source: tr.Source.Model, Target: tr.Content.Model, By: tr.By,
		},
		Approved: tr.State == localizationdomain.StateApproved,
	}, nil
}
