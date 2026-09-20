// Package qa is `glossa check`: structural QA over a project snapshot
// (product intent §29.1, §30). Each layer is a Checker, so later layers
// (source copy lint §28, terminology, usage) plug in without changing
// the command; a Policy decides which findings fail the check.
package qa

import (
	"sort"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
)

// The policy and its vocabulary live in the kernel, because the Glossa
// PR check decides the same question on the server and must decide it
// the same way (RFC 0004 §6.4). These are aliases, not copies: there is
// one policy, and `glossa check` and the pull request share it.

// Severity ranks a finding.
type Severity = checkpolicy.Severity

// Severities.
const (
	Error   = checkpolicy.Error
	Warning = checkpolicy.Warning
)

// Finding codes Glossa adds to the kernel's (compat findings keep the
// kernel's codes, e.g. missing-argument).
const (
	CodeInvalidMessage      = checkpolicy.CodeInvalidMessage
	CodeInvalidTranslation  = checkpolicy.CodeInvalidTranslation
	CodeMissingTranslation  = checkpolicy.CodeMissingTranslation
	CodeOutdatedTranslation = checkpolicy.CodeOutdatedTranslation
	CodeUnknownKey          = checkpolicy.CodeUnknownKey
	CodeMissingLocale       = checkpolicy.CodeMissingLocale
)

// Finding is one problem.
type Finding struct {
	// Check is the checker that found it (structure, arguments, completeness).
	Check    string   `json:"check"`
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Locale   string   `json:"locale,omitempty"`
	Key      string   `json:"key,omitempty"`
	Subject  string   `json:"subject,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	Message  string   `json:"message"`
	// Where is the local file, when the finding comes from one.
	Where string `json:"where,omitempty"`
}

// Policy decides what fails a check: `require_complete` and `fail_on`,
// shared with the server's PR check.
type Policy = checkpolicy.Policy

// Checker is one layer of QA.
type Checker interface {
	Name() string
	Check(s *snapshot.Snapshot, p Policy) []Finding
}

// Default is the structural QA of M1.
func Default() []Checker {
	return []Checker{structure{}, arguments{}, completeness{}}
}

// Precomputed is a Checker reporting findings computed elsewhere, such
// as the terminology layer, which asks the server.
func Precomputed(name string, fs []Finding) Checker { return precomputed{name: name, findings: fs} }

type precomputed struct {
	name     string
	findings []Finding
}

func (c precomputed) Name() string { return c.name }

func (c precomputed) Check(*snapshot.Snapshot, Policy) []Finding { return c.findings }

// LocaleReport summarizes one locale.
type LocaleReport struct {
	Code     string `json:"code"`
	IsSource bool   `json:"is_source"`
	Required bool   `json:"required"`
	Messages int    `json:"messages"`
	// Translated counts messages with a usable translation (any review
	// state but rejected).
	Translated int  `json:"translated"`
	Missing    int  `json:"missing"`
	Outdated   int  `json:"outdated"`
	Errors     int  `json:"errors"`
	Warnings   int  `json:"warnings"`
	Complete   bool `json:"complete"`
}

// Report is a check's result.
type Report struct {
	Origin   string         `json:"origin"`
	Messages int            `json:"messages"`
	Invalid  int            `json:"invalid_messages"`
	Locales  []LocaleReport `json:"locales"`
	Findings []Finding      `json:"findings"`
	Errors   int            `json:"errors"`
	Warnings int            `json:"warnings"`
	Passed   bool           `json:"passed"`
}

// Run checks s with the checkers and summarizes.
func Run(s *snapshot.Snapshot, p Policy, checkers ...Checker) Report {
	r := Report{Origin: s.Origin, Messages: len(s.Messages), Findings: []Finding{}, Passed: true}
	for _, c := range checkers {
		r.Findings = append(r.Findings, c.Check(s, p)...)
	}
	sortFindings(r.Findings)
	for _, m := range s.Messages {
		if m.Invalid != nil {
			r.Invalid++
		}
	}
	perLocale := map[string]*LocaleReport{}
	for _, l := range s.Locales {
		lr := &LocaleReport{Code: l.Code, IsSource: l.IsSource, Required: !l.IsSource && p.Requires(l.Code), Messages: len(s.Messages)}
		if !l.IsSource {
			lr.Translated = translated(s, l.Code)
		} else {
			lr.Translated = len(s.Messages)
		}
		perLocale[l.Code] = lr
	}
	for _, f := range r.Findings {
		if f.Severity == Error {
			r.Errors++
		} else {
			r.Warnings++
		}
		if p.Fails(f.Severity) {
			r.Passed = false
		}
		lr, ok := perLocale[f.Locale]
		if !ok {
			continue
		}
		switch f.Code {
		case CodeMissingTranslation:
			lr.Missing++
		case CodeOutdatedTranslation:
			lr.Outdated++
		}
		if f.Severity == Error {
			lr.Errors++
		} else {
			lr.Warnings++
		}
	}
	for _, l := range s.Locales {
		lr := perLocale[l.Code]
		lr.Complete = lr.Missing == 0
		r.Locales = append(r.Locales, *lr)
	}
	return r
}

func translated(s *snapshot.Snapshot, locale string) int {
	n := 0
	for _, m := range s.Messages {
		if t, ok := s.Translations[locale][m.Key]; ok && t.State != "rejected" {
			n++
		}
	}
	return n
}

func sortFindings(fs []Finding) {
	rank := map[Severity]int{Error: 0, Warning: 1}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Locale != b.Locale {
			return a.Locale < b.Locale
		}
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		return a.Key < b.Key
	})
}

// ── structure ───────────────────────────────────────────────────────

// structure: every source message and translation parses into a valid
// MessageFormat 2 model.
type structure struct{}

func (structure) Name() string { return "structure" }

func (structure) Check(s *snapshot.Snapshot, _ Policy) []Finding {
	var out []Finding
	for _, m := range s.Messages {
		if m.Invalid != nil {
			out = append(out, Finding{Check: "structure", Code: CodeInvalidMessage, Severity: Error,
				Locale: s.SourceLocale, Key: m.Key, Detail: m.Invalid.Code,
				Message: "invalid message: " + m.Invalid.Detail, Where: m.File})
		}
	}
	for _, l := range s.TargetLocales() {
		for _, t := range sortedTranslations(s, l.Code) {
			if t.Invalid != nil {
				out = append(out, Finding{Check: "structure", Code: CodeInvalidTranslation, Severity: Error,
					Locale: l.Code, Key: t.Key, Detail: t.Invalid.Code,
					Message: "invalid translation: " + t.Invalid.Detail, Where: t.File})
			}
		}
	}
	return out
}

func sortedTranslations(s *snapshot.Snapshot, locale string) []snapshot.Translation {
	trs := s.Translations[locale]
	out := make([]snapshot.Translation, 0, len(trs))
	for _, t := range trs {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// ── arguments ───────────────────────────────────────────────────────

// arguments: every translation is structurally compatible with its
// current source (messageformat.CheckCompat), plus the warnings the
// server stored that only it can compute (max-length-exceeded).
type arguments struct{}

func (arguments) Name() string { return "arguments" }

func (arguments) Check(s *snapshot.Snapshot, _ Policy) []Finding {
	var out []Finding
	for _, l := range s.TargetLocales() {
		for _, t := range sortedTranslations(s, l.Code) {
			m, ok := s.Message(t.Key)
			if !ok || m.Model == nil || t.Model == nil || t.State == "rejected" {
				continue
			}
			seen := map[string]bool{}
			for _, f := range mf.CheckCompat(*m.Model, *t.Model, l.Code) {
				seen[string(f.Code)] = true
				out = append(out, fromKernel(f, l.Code, t))
			}
			for _, w := range t.Warnings {
				if !seen[string(w.Code)] {
					out = append(out, fromKernel(w, l.Code, t))
				}
			}
		}
	}
	return out
}

func fromKernel(f mf.Finding, locale string, t snapshot.Translation) Finding {
	sev := Warning
	if f.Severity == mf.SeverityError {
		sev = Error
	}
	return Finding{Check: "arguments", Code: string(f.Code), Severity: sev, Locale: locale, Key: t.Key,
		Subject: f.Subject, Detail: f.Detail, Message: f.Message, Where: t.File}
}

// ── completeness ────────────────────────────────────────────────────

// completeness: every active message has a translation in every locale
// (required locales: the policy's missing_translations, error by
// default; others: warning), made against the current source (outdated:
// warning), and no local translation names an unknown message.
type completeness struct{}

func (completeness) Name() string { return "completeness" }

func (completeness) Check(s *snapshot.Snapshot, p Policy) []Finding {
	var out []Finding
	have := map[string]bool{}
	for _, l := range s.Locales {
		have[l.Code] = true
	}
	for _, l := range p.RequireComplete {
		if !have[l] {
			out = append(out, Finding{Check: "completeness", Code: CodeMissingLocale, Severity: Error, Locale: l,
				Message: "required locale isn't in the project"})
		}
	}
	for _, l := range s.TargetLocales() {
		// The policy decides, not the required list alone: a project can
		// require every locale and still only warn about untranslated
		// keys (checkpolicy.Policy.MissingTranslations).
		sev := p.Severity(l.Code)
		trs := s.Translations[l.Code]
		for _, m := range s.Messages {
			t, ok := trs[m.Key]
			switch {
			case !ok || t.State == "rejected":
				msg := "missing translation"
				if ok {
					msg = "translation rejected in review"
				}
				out = append(out, Finding{Check: "completeness", Code: CodeMissingTranslation, Severity: sev,
					Locale: l.Code, Key: m.Key, Message: msg, Where: l.File})
			case t.Outdated:
				out = append(out, Finding{Check: "completeness", Code: CodeOutdatedTranslation, Severity: Warning,
					Locale: l.Code, Key: m.Key, Message: "made against an older source revision; the source changed since"})
			}
		}
		for _, t := range sortedTranslations(s, l.Code) {
			if _, ok := s.Message(t.Key); !ok {
				out = append(out, Finding{Check: "completeness", Code: CodeUnknownKey, Severity: Warning,
					Locale: l.Code, Key: t.Key, Message: "no source message has this ID", Where: t.File})
			}
		}
	}
	return out
}
