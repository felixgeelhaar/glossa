// Package snapshot adapts Catalog's and Localization's application
// services to Quality's Snapshot port: the project as the layers read
// it, plus the policy that grades them.
//
// It reads the two snapshots the release build already reads —
// Catalog's active source and Localization's translations — through
// their own application services, as RFC 0002 §4 requires. Each keeps
// checking the caller's permissions, so a check runs with exactly what
// its caller could have read through the API and nothing more: an MCP
// session on a read-only token, `catalog.read` and `translations.read`,
// sees the same project a person with those permissions sees.
//
// Nothing here decides anything. The layers are Quality's, the policy
// is the project's, and this package only puts the catalog in front of
// them in the shape they expect.
package snapshot

import (
	"context"
	"errors"

	"github.com/google/uuid"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	catalogdomain "github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	localizationapp "github.com/felixgeelhaar/glossa/platform/internal/localization/app"
	localizationdomain "github.com/felixgeelhaar/glossa/platform/internal/localization/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/layers"
)

// Origin says where a checked project was read from. `glossa check`
// reports "local"; a run on the server reports this.
const Origin = "server"

// Port implements app.Snapshot over the two contexts that hold a
// project's text.
type Port struct {
	catalog      *catalogapp.Service
	localization *localizationapp.Service
}

// New returns the port.
func New(c *catalogapp.Service, l *localizationapp.Service) *Port {
	return &Port{catalog: c, localization: l}
}

var _ app.Snapshot = (*Port)(nil)

// Snapshot implements app.Snapshot.
func (p *Port) Snapshot(ctx context.Context, project uuid.UUID) (app.ProjectSnapshot, error) {
	src, err := p.catalog.ReleaseSource(ctx, catalogdomain.ProjectID(project))
	if err != nil {
		return app.ProjectSnapshot{}, notFound(err)
	}
	// Every review state but rejected: a check grades what the project
	// has, including text still waiting for a reviewer, exactly as
	// `glossa check` grades the catalogs on disk. Rejected text is not
	// a translation, and the completeness layer must see its absence.
	states := []localizationdomain.ReviewState{
		localizationdomain.StateDraft, localizationdomain.StateNeedsReview, localizationdomain.StateApproved,
	}
	trs, err := p.localization.ReleaseTranslations(ctx, project, states)
	if err != nil {
		return app.ProjectSnapshot{}, notFound(err)
	}
	out := &layers.Project{
		Origin:       Origin,
		SourceLocale: trs.SourceLocale.String(),
		Translations: make(map[string]map[string]layers.Translation, len(trs.Translations)),
	}
	for _, l := range trs.Locales {
		out.Locales = append(out.Locales, layers.Locale{Code: l.Code.String(), IsSource: l.IsSource})
	}
	keys := make(map[uuid.UUID]catalogdomain.Message, len(src.Messages))
	for _, m := range src.Messages {
		keys[uuid.UUID(m.ID)] = m
		out.Messages = append(out.Messages, layers.Message{
			ID: uuid.UUID(m.ID).String(), Key: string(m.Key), Namespace: string(m.Namespace),
			Revision: m.Revision, Model: model(m.Source.Model),
		})
	}
	for locale, byID := range trs.Translations {
		texts := make(map[string]layers.Translation, len(byID))
		for id, v := range byID {
			// A translation of a message the source snapshot does not
			// carry is a translation of an obsolete message: not part of
			// what ships, and not part of what is checked.
			m, ok := keys[id]
			if !ok {
				continue
			}
			texts[string(m.Key)] = translation(string(m.Key), locale.String(), m.Revision, v)
		}
		out.Translations[locale.String()] = texts
	}
	return app.ProjectSnapshot{Project: out, Policy: src.Project.Settings.Policy()}, nil
}

// translation is one stored translation as a layer reads it.
func translation(key, locale string, sourceRevision int, v localizationapp.TranslationView) layers.Translation {
	t := layers.Translation{
		Key: key, Locale: locale, Model: model(v.Content.Model), State: string(v.State),
		Revision: v.ID.String(), SourceRevision: v.SourceRevision,
		Outdated: v.Translation.Outdated(sourceRevision),
	}
	// The warnings Localization stored with the text are findings only
	// it can compute (max-length among them), and the parity layer
	// reports them rather than recomputing them.
	for _, w := range v.Warnings {
		t.Warnings = append(t.Warnings, w)
	}
	return t
}

// model returns the parsed message, or nil where there is none — a
// layer reads a nil model as text it cannot check. Stored content is
// restored with its model, so nil here means the row never had one.
func model(m mf.Message) *mf.Message {
	if m.Type == "" {
		return nil
	}
	return &m
}

// notFound translates the contexts' own not-found errors into Quality's.
func notFound(err error) error {
	if errors.Is(err, catalogapp.ErrNotFound) || errors.Is(err, localizationapp.ErrNotFound) {
		return app.ErrProjectNotFound
	}
	return err
}
