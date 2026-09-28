package layers

import (
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// Completeness checks that every active message has a translation in
// every locale — required locales (the policy's missing_translations,
// error by default) and others (warning) — made against the current
// source (outdated: warning), and that no translation names a message
// the catalog does not have.
type Completeness struct{}

// Layer implements Checker.
func (Completeness) Layer() domain.Layer { return domain.LayerCompleteness }

// Check implements Checker.
func (Completeness) Check(p *Project, policy checkpolicy.Policy) []domain.Finding {
	var out []domain.Finding
	have := map[string]bool{}
	for _, l := range p.Locales {
		have[l.Code] = true
	}
	for _, l := range policy.RequireComplete {
		if have[l] {
			continue
		}
		out = append(out, domain.New(domain.Finding{
			Layer: domain.LayerCompleteness, Code: checkpolicy.CodeMissingLocale, Severity: domain.Error,
			Locus: domain.Locus{Locale: l}, Message: "required locale isn't in the project",
		}))
	}
	for _, l := range p.TargetLocales() {
		// The policy decides, not the required list alone: a project can
		// require every locale and still only warn about untranslated
		// keys (checkpolicy.Policy.MissingTranslations).
		severity := policy.Severity(l.Code)
		trs := p.Translations[l.Code]
		for _, m := range p.Messages {
			t, ok := trs[m.Key]
			switch {
			case !ok || t.State == "rejected":
				message := "missing translation"
				if ok {
					message = "translation rejected in review"
				}
				out = append(out, domain.New(domain.Finding{
					Layer: domain.LayerCompleteness, Code: checkpolicy.CodeMissingTranslation, Severity: severity,
					Locus: domain.Locus{
						Message: m.ID, Key: m.Key, Locale: l.Code, Namespace: m.Namespace, File: l.File,
					},
					Message: message,
				}))
			case t.Outdated:
				out = append(out, domain.New(domain.Finding{
					Layer: domain.LayerCompleteness, Code: checkpolicy.CodeOutdatedTranslation, Severity: domain.Warning,
					Locus: domain.Locus{
						Message: m.ID, Key: m.Key, Locale: l.Code, Namespace: m.Namespace, Revision: t.Revision,
					},
					Message:        "made against an older source revision; the source changed since",
					SourceRevision: sourceRevision(t),
					Fix:            &domain.Fix{Kind: domain.FixAdoptSourceChange},
				}))
			}
		}
		for _, t := range p.SortedTranslations(l.Code) {
			if _, ok := p.Message(t.Key); ok {
				continue
			}
			out = append(out, domain.New(domain.Finding{
				Layer: domain.LayerCompleteness, Code: checkpolicy.CodeUnknownKey, Severity: domain.Warning,
				Locus:   domain.Locus{Key: t.Key, Locale: l.Code, Revision: t.Revision, File: t.File},
				Message: "no source message has this ID",
			}))
		}
	}
	return out
}
