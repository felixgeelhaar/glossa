// Package terminology is `glossa terms check` and the optional
// terminology layer of `glossa check` (intent §29.3, RFC 0003 §2.2): the
// server's terminology QA over a project's translations —
// term_forbidden when a translation uses a forbidden (error) or
// deprecated (warning) term, term_missing (warning) when a source term's
// preferred and admitted translations are all absent. The server runs
// it over every translation (GET …/terminology-findings) a page at a
// time; this package sums the pages into a report.
package terminology

import (
	"context"
	"sort"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/qa"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/remote"
)

// CheckName is the qa check the findings belong to.
const CheckName = "terminology"

// maxLocales is the server's limit of locales per project check.
const maxLocales = 20

// Fetcher pages through the server's project check
// (remote.Client.ProjectTermFindings).
type Fetcher func(ctx context.Context, q remote.TermFindingsQuery, fn func(remote.TermFindingsPage) error) error

// Finding is one terminology problem in a translation.
type Finding struct {
	Code     string `json:"code"`     // term_forbidden or term_missing
	Severity string `json:"severity"` // error or warning
	Locale   string `json:"locale"`
	Key      string `json:"key"`
	// Side is where the span is: source (term_missing: the source term)
	// or target (term_forbidden: the term used).
	Side  string `json:"side"`
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	// Suggestions are the preferred and admitted target terms.
	Suggestions []string `json:"suggestions"`
	ConceptID   string   `json:"concept_id"`
	TermID      string   `json:"term_id"`
	Message     string   `json:"message"`
}

// LocaleReport summarizes one locale.
type LocaleReport struct {
	Code     string `json:"code"`
	Checked  int    `json:"checked"`
	Errors   int    `json:"errors"`
	Warnings int    `json:"warnings"`
}

// Report is a terminology check's result.
type Report struct {
	Locales  []LocaleReport `json:"locales"`
	Findings []Finding      `json:"findings"`
	Checked  int            `json:"checked"`
	Errors   int            `json:"errors"`
	Warnings int            `json:"warnings"`
}

// Options select what Run checks.
type Options struct {
	// Locales are the target locales, in report order.
	Locales []string
	// States are the review states checked; empty means every state
	// but rejected.
	States []string
}

// Run has the server check every selected translation — up to 20
// locales per request, a page at a time — and sums the pages. Findings
// come ordered by locale, key and position.
func Run(ctx context.Context, opts Options, fetch Fetcher) (Report, error) {
	r := Report{Locales: []LocaleReport{}, Findings: []Finding{}}
	perLocale := map[string]*LocaleReport{}
	for _, l := range opts.Locales {
		perLocale[l] = &LocaleReport{Code: l}
	}
	for start := 0; start < len(opts.Locales); start += maxLocales {
		q := remote.TermFindingsQuery{Locales: opts.Locales[start:min(start+maxLocales, len(opts.Locales))], States: opts.States}
		err := fetch(ctx, q, func(page remote.TermFindingsPage) error {
			for l, n := range page.Checked {
				if lr, ok := perLocale[l]; ok {
					lr.Checked += n
				}
			}
			for _, it := range page.Items {
				lr, ok := perLocale[it.Locale]
				if !ok {
					continue
				}
				for _, f := range it.Findings {
					r.Findings = append(r.Findings, Finding{Code: string(f.Code), Severity: string(f.Severity), Locale: it.Locale,
						Key: it.MessageKey, Side: string(f.Side), Text: f.Text, Start: f.Start, End: f.End,
						Suggestions: nonNil(f.Suggestions), ConceptID: f.ConceptId, TermID: f.TermId, Message: f.Message})
					if f.Severity == "error" {
						lr.Errors++
					} else {
						lr.Warnings++
					}
				}
			}
			return nil
		})
		if err != nil {
			return Report{}, err
		}
	}
	rank := map[string]int{}
	for i, l := range opts.Locales {
		rank[l] = i
	}
	sort.SliceStable(r.Findings, func(a, b int) bool {
		fa, fb := r.Findings[a], r.Findings[b]
		if fa.Locale != fb.Locale {
			return rank[fa.Locale] < rank[fb.Locale]
		}
		return fa.Key < fb.Key
	})
	for _, l := range opts.Locales {
		lr := perLocale[l]
		r.Locales = append(r.Locales, *lr)
		r.Checked += lr.Checked
		r.Errors += lr.Errors
		r.Warnings += lr.Warnings
	}
	return r, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// QA turns the findings into `glossa check` findings.
func (r Report) QA() []qa.Finding {
	out := make([]qa.Finding, 0, len(r.Findings))
	for _, f := range r.Findings {
		sev := qa.Warning
		if f.Severity == "error" {
			sev = qa.Error
		}
		out = append(out, qa.Finding{Check: CheckName, Code: f.Code, Severity: sev, Locale: f.Locale, Key: f.Key,
			Subject: f.Text, Message: f.Message})
	}
	return out
}
