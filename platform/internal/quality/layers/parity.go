package layers

import (
	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// Parity checks that every translation is structurally compatible with
// its current source — arguments, selectors, plural categories for the
// target locale and markup — plus the warnings the server stored that
// only it can compute (max-length-exceeded).
//
// It was called `arguments` in M1 and keeps every one of its codes,
// which stay the MessageFormat kernel's (RFC 0005 §3.1).
type Parity struct{}

// Layer implements Checker.
func (Parity) Layer() domain.Layer { return domain.LayerParity }

// Check implements Checker.
func (Parity) Check(p *Project, _ checkpolicy.Policy) []domain.Finding {
	var out []domain.Finding
	for _, l := range p.TargetLocales() {
		for _, t := range p.SortedTranslations(l.Code) {
			m, ok := p.Message(t.Key)
			if !ok || m.Model == nil || t.Model == nil || t.State == "rejected" {
				continue
			}
			seen := map[string]bool{}
			for _, f := range mf.CheckCompat(*m.Model, *t.Model, l.Code) {
				seen[string(f.Code)] = true
				out = append(out, FromKernel(f, l.Code, m, t))
			}
			// A warning the kernel cannot recompute here — max_length is
			// the server's, because only it holds the constraint — is
			// reported as the server left it.
			for _, w := range t.Warnings {
				if !seen[string(w.Code)] {
					out = append(out, FromKernel(w, l.Code, m, t))
				}
			}
		}
	}
	return out
}

// FromKernel is a MessageFormat finding as the parity layer reports it.
//
// It is a constructor and not a conversion: the kernel's subject and
// qualifier survive, and the locus says which message, which locale and
// which revision — all of which the conversion this replaces dropped on
// the way to the server or to the CLI.
func FromKernel(f mf.Finding, locale string, m *Message, t Translation) domain.Finding {
	severity := domain.Warning
	if f.Severity == mf.SeverityError {
		severity = domain.Error
	}
	locus := domain.Locus{
		Key: t.Key, Locale: locale, Revision: t.Revision, File: t.File,
	}
	if m != nil {
		locus.Message, locus.Namespace = m.ID, m.Namespace
	}
	return domain.New(domain.Finding{
		Layer: domain.LayerParity, Code: string(f.Code), Severity: severity, Locus: locus,
		Message: f.Message, Subject: f.Subject, Detail: f.Detail, SourceRevision: sourceRevision(t),
	})
}
