package sources

import (
	"context"

	"github.com/google/uuid"

	localizationapp "github.com/felixgeelhaar/glossa/platform/internal/localization/app"
	"github.com/felixgeelhaar/glossa/platform/internal/mcp/tools"
)

// Translations adapts Localization's application service to
// tools.Translations.
type Translations struct{ localization *localizationapp.Service }

// NewTranslations returns the adapter.
func NewTranslations(s *localizationapp.Service) *Translations { return &Translations{localization: s} }

var _ tools.Translations = (*Translations)(nil)

// Translation implements tools.Translations.
func (a *Translations) Translation(
	ctx context.Context, project uuid.UUID, key, locale string,
) (tools.Translation, error) {
	v, err := a.localization.GetTranslation(ctx, project, key, locale)
	if err != nil {
		return tools.Translation{}, notFound(err, localizationapp.ErrNotFound, localizationapp.ErrLocaleNotFound)
	}
	out := tools.Translation{
		MessageID: v.MessageID.String(), Key: key, Locale: v.Locale.String(),
		Text: v.Content.Text, Syntax: string(v.Content.Syntax), State: string(v.State),
		Origin: string(v.Origin), By: v.By, SourceRevision: v.SourceRevision,
		CurrentSourceRevision: v.CurrentSourceRevision, Outdated: v.Outdated(), Revision: v.Revision,
	}
	// Only the codes: the warnings' prose is Quality's to report, and
	// findings_list returns it properly.
	for _, w := range v.Warnings {
		out.Warnings = append(out.Warnings, string(w.Code))
	}
	return out, nil
}
