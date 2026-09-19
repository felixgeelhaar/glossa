// Package terminology is `glossa terms check` and the optional
// terminology layer of `glossa check` (intent §29.3, RFC 0003 §2.2): it
// runs the server's terminology QA over a project snapshot's
// translations — term_forbidden when a translation uses a forbidden
// (error) or deprecated (warning) term, term_missing (warning) when a
// source term's preferred and admitted translations are all absent.
package terminology

import (
	"context"
	"sort"
	"sync"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/qa"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/remote"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
)

// CheckName is the qa check the findings belong to.
const CheckName = "terminology"

// Checker checks one translation (the server's terminology-checks).
type Checker func(ctx context.Context, req remote.TerminologyRequest) (remote.TerminologyCheck, error)

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
	// Locales are the target locales; empty means every one.
	Locales []string
	// States are the review states checked; empty means every state
	// but rejected.
	States []string
	// Project is the project ID, so project-scoped terms apply.
	Project string
	// Concurrency bounds requests in flight (default 8).
	Concurrency int
}

type job struct {
	locale string
	key    string
	req    remote.TerminologyRequest
}

// Run checks every selected translation of s whose message and text
// parse. Findings come ordered by locale, key and position.
func Run(ctx context.Context, s *snapshot.Snapshot, opts Options, check Checker) (Report, error) {
	r := Report{Locales: []LocaleReport{}, Findings: []Finding{}}
	var jobs []job
	perLocale := map[string]*LocaleReport{}
	for _, l := range s.TargetLocales() {
		if len(opts.Locales) > 0 && !has(opts.Locales, l.Code) {
			continue
		}
		lr := &LocaleReport{Code: l.Code}
		perLocale[l.Code] = lr
		for _, m := range s.Messages {
			t, ok := s.Translations[l.Code][m.Key]
			if !ok || !selected(t.State, opts.States) {
				continue
			}
			req, ok := request(s.SourceLocale, l.Code, opts.Project, m, t)
			if !ok {
				continue
			}
			jobs = append(jobs, job{locale: l.Code, key: m.Key, req: req})
			lr.Checked++
		}
	}
	results, err := runAll(ctx, jobs, opts.Concurrency, check)
	if err != nil {
		return r, err
	}
	for i, res := range results {
		j := jobs[i]
		for _, f := range res.Findings {
			r.Findings = append(r.Findings, Finding{Code: string(f.Code), Severity: string(f.Severity), Locale: j.locale, Key: j.key,
				Side: string(f.Side), Text: f.Text, Start: f.Start, End: f.End, Suggestions: nonNil(f.Suggestions),
				ConceptID: f.ConceptId, TermID: f.TermId, Message: f.Message})
			if f.Severity == "error" {
				perLocale[j.locale].Errors++
				r.Errors++
			} else {
				perLocale[j.locale].Warnings++
				r.Warnings++
			}
		}
	}
	sort.SliceStable(r.Findings, func(a, b int) bool {
		fa, fb := r.Findings[a], r.Findings[b]
		if fa.Locale != fb.Locale {
			return fa.Locale < fb.Locale
		}
		return fa.Key < fb.Key
	})
	for _, l := range s.TargetLocales() {
		if lr, ok := perLocale[l.Code]; ok {
			r.Locales = append(r.Locales, *lr)
			r.Checked += lr.Checked
		}
	}
	return r, nil
}

func has(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func selected(state string, states []string) bool {
	if len(states) == 0 {
		return state != "rejected"
	}
	return has(states, state)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// request builds the check for one translation: the texts as they are
// when both share a syntax, else both as MF2 from their models.
func request(sourceLocale, locale, project string, m snapshot.Message, t snapshot.Translation) (remote.TerminologyRequest, bool) {
	if m.Model == nil || t.Model == nil {
		return remote.TerminologyRequest{}, false // structural QA reports these
	}
	src, tgt, syntax := m.Text, t.Text, m.Syntax
	if m.Syntax != t.Syntax || syntax == "" {
		var err error
		if src, err = mf.Stringify(*m.Model); err != nil {
			return remote.TerminologyRequest{}, false
		}
		if tgt, err = mf.Stringify(*t.Model); err != nil {
			return remote.TerminologyRequest{}, false
		}
		syntax = "mf2"
	}
	st := remote.Syntax(syntax)
	req := remote.TerminologyRequest{Source: src, Target: tgt, SourceLocale: sourceLocale, TargetLocale: locale, Syntax: &st}
	if project != "" {
		req.ProjectId = &project
	}
	return req, true
}

// runAll checks jobs with at most n requests in flight; results are in
// job order. The first error cancels the rest.
func runAll(ctx context.Context, jobs []job, n int, check Checker) ([]remote.TerminologyCheck, error) {
	if n <= 0 {
		n = 8
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]remote.TerminologyCheck, len(jobs))
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	next := make(chan int)
	for range min(n, max(len(jobs), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				res, err := check(ctx, jobs[i].req)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					mu.Unlock()
					continue
				}
				results[i] = res
			}
		}()
	}
feed:
	for i := range jobs {
		select {
		case next <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(next)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return results, ctx.Err()
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
