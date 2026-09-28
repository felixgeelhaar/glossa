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
	// so a reader can tell "clean" from "not looked at". A layer the
	// policy switched off is not in the list.
	Layers     []domain.Layer
	Conclusion domain.Conclusion
	// Decisions are the policy's decision per finding, in the order of
	// Findings: which rule gave it this severity, and whether the rule
	// was enforcing. It is what `glossa check --explain-policy` prints
	// (RFC 0005 §4.3).
	Decisions []checkpolicy.Decision
	// PolicyVersion is the version of the document the run graded
	// itself against; 0 for a policy that has none.
	PolicyVersion int
	// Skipped are the layers the policy switched off, named rather than
	// silently dropped: a check may never lose a layer in silence.
	Skipped []domain.Layer
}

// Passed reports whether the run's conclusion lets a build through.
func (r Report) Passed() bool { return r.Conclusion != domain.ConclusionFailure }

// Run checks p with the checkers and sums what they found.
//
// It is the branch form of RunIn: a check in CI runs in no environment
// at all, which is where the policy's environment blocks and any rule
// naming an environment say nothing.
func Run(p *layers.Project, policy checkpolicy.Policy, checkers ...layers.Checker) Report {
	return RunIn(p, policy, "", checkers...)
}

// RunIn checks p with the checkers in environment env and sums what
// they found.
//
// The policy is the evaluator throughout: it decides which layers run
// at all (a rule may switch one off), what severity each finding has
// here, and what fails. A run never decides any of that for itself.
func RunIn(p *layers.Project, policy checkpolicy.Policy, env string, checkers ...layers.Checker) Report {
	// The environment is resolved once, here: below this line a layer
	// asks whether a locale must be complete and gets this
	// environment's answer without having to know there are others.
	policy = policy.In(env)
	r := Report{Origin: p.Origin, Messages: len(p.Messages), Findings: []domain.Finding{}}
	for _, c := range checkers {
		if !domain.Computes(policy, env, c.Layer()) {
			r.Skipped = append(r.Skipped, c.Layer())
			continue
		}
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
			Code: l.Code, IsSource: l.IsSource, Required: !l.IsSource && policy.RequiresIn(env, l.Code),
			Messages: len(p.Messages),
		}
		if l.IsSource {
			lr.Translated = len(p.Messages)
		} else {
			lr.Translated = translated(p, l.Code)
		}
		perLocale[l.Code] = lr
	}
	// The policy grades what the layers found: the severity of a finding
	// is the document's answer for this locale, namespace and
	// environment, and a finding a rule switched off never reaches the
	// report.
	ev := domain.Evaluate(policy, env, r.Findings)
	r.Findings, r.Counts, r.Conclusion = ev.Findings(), ev.Counts, ev.Conclusion
	r.PolicyVersion = ev.PolicyVersion
	r.Decisions = make([]checkpolicy.Decision, 0, len(ev.Graded))
	for _, g := range ev.Graded {
		r.Decisions = append(r.Decisions, g.Decision)
	}
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
