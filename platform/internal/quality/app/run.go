// Package app runs the Quality context's layers over a project and
// sums what they found (RFC 0005 §2.2): one run, one verdict, the same
// on every surface.
//
// The check policy stays the evaluator. A run counts its findings and
// asks the policy what fails; it never decides that for itself, because
// the terminal and the pull request must reach the same verdict from
// the same policy and that is the one property `checkpolicy` was
// created to protect.
package app

import (
	"sort"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/layers"
)

// LocaleReport summarizes one locale.
type LocaleReport struct {
	Code     string
	IsSource bool
	Required bool
	Messages int
	// Translated counts messages with a usable translation (any review
	// state but rejected).
	Translated int
	Missing    int
	Outdated   int
	Errors     int
	Warnings   int
	Waived     int
	Complete   bool
}

// Report is one run's result: what was checked, what was found, and
// what the policy made of it.
type Report struct {
	Origin string
	// Messages is the project's active messages.
	Messages int
	// Invalid counts the source messages that don't parse.
	Invalid  int
	Locales  []LocaleReport
	Findings []domain.Finding
	Counts   domain.Counts
	// Layers are the layers the run computed, in the order it ran them,
	// so a reader can tell "clean" from "not looked at".
	Layers     []domain.Layer
	Conclusion domain.Conclusion
}

// Passed reports whether the run's conclusion lets a build through.
func (r Report) Passed() bool { return r.Conclusion != domain.ConclusionFailure }

// Run checks p with the checkers and sums what they found.
func Run(p *layers.Project, policy checkpolicy.Policy, checkers ...layers.Checker) Report {
	r := Report{Origin: p.Origin, Messages: len(p.Messages), Findings: []domain.Finding{}}
	for _, c := range checkers {
		r.Layers = append(r.Layers, c.Layer())
		r.Findings = append(r.Findings, c.Check(p, policy)...)
	}
	sortFindings(r.Findings)
	for _, m := range p.Messages {
		if m.Invalid != nil {
			r.Invalid++
		}
	}
	perLocale := map[string]*LocaleReport{}
	for _, l := range p.Locales {
		lr := &LocaleReport{
			Code: l.Code, IsSource: l.IsSource, Required: !l.IsSource && policy.Requires(l.Code),
			Messages: len(p.Messages),
		}
		if l.IsSource {
			lr.Translated = len(p.Messages)
		} else {
			lr.Translated = translated(p, l.Code)
		}
		perLocale[l.Code] = lr
	}
	r.Counts, r.Conclusion = domain.Conclude(policy, r.Findings)
	for _, f := range r.Findings {
		lr, ok := perLocale[f.Locus.Locale]
		if !ok {
			continue
		}
		switch f.Code {
		case checkpolicy.CodeMissingTranslation:
			lr.Missing++
		case checkpolicy.CodeOutdatedTranslation:
			lr.Outdated++
		}
		switch f.Severity {
		case domain.Waived:
			lr.Waived++
		case domain.Error:
			lr.Errors++
		default:
			lr.Warnings++
		}
	}
	for _, l := range p.Locales {
		lr := perLocale[l.Code]
		lr.Complete = lr.Missing == 0
		r.Locales = append(r.Locales, *lr)
	}
	return r
}

func translated(p *layers.Project, locale string) int {
	n := 0
	for _, m := range p.Messages {
		if t, ok := p.Translations[locale][m.Key]; ok && t.State != "rejected" {
			n++
		}
	}
	return n
}

// sortFindings puts a report in reading order: by locale, errors before
// warnings before waived, then by key.
func sortFindings(fs []domain.Finding) {
	rank := map[domain.Severity]int{domain.Error: 0, domain.Warning: 1, domain.Waived: 2}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Locus.Locale != b.Locus.Locale {
			return a.Locus.Locale < b.Locus.Locale
		}
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		return a.Locus.Key < b.Locus.Key
	})
}
