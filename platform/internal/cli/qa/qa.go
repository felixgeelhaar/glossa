// Package qa is `glossa check`'s side of the Quality context: it turns
// the CLI's snapshot into the project a layer checks, runs the layers,
// and renders the result as `glossa check --json` has always rendered
// it.
//
// The QA itself moved to internal/quality in M4 (RFC 0005 §14
// decision 1): the layers, the finding and the run all live there now,
// so `glossa check`, the Glossa pull-request check and the server's
// jobs share one implementation and cannot disagree. What is left here
// is the adapter — a snapshot in, the command's wire shape out.
//
// The wire shape is deliberately unchanged. `glossa.cli.check/v1` is
// the command's own contract, versioned on its own, and RFC 0005 §13
// wave 3 rebuilds the command on the Quality library; until then a
// finding prints and serializes exactly as it did in M3.
package qa

import (
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	qualityapp "github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/layers"
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

// Finding is one problem as `glossa check` prints and serializes it: a
// flattened quality finding, with the fields the command has always
// shown.
type Finding struct {
	// Check is the layer that found it, under the name the command has
	// always printed.
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
type Checker = layers.Checker

// Default is the deterministic QA every check runs.
func Default() []Checker { return layers.Default() }

// Precomputed is a Checker reporting findings computed elsewhere, such
// as the terminology layer, which asks the server.
func Precomputed(layer domain.Layer, fs []domain.Finding) Checker {
	return layers.Precomputed(layer, fs)
}

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
	return render(qualityapp.Run(Project(s), p, checkers...))
}

// Project is the snapshot as a layer sees it. The CLI's snapshot holds
// no message IDs — it is read by key — so a finding from an offline
// check is fingerprinted by key (domain.Fingerprint).
func Project(s *snapshot.Snapshot) *layers.Project {
	p := &layers.Project{
		Origin: s.Origin, SourceLocale: s.SourceLocale,
		Translations: make(map[string]map[string]layers.Translation, len(s.Translations)),
	}
	for _, l := range s.Locales {
		p.Locales = append(p.Locales, layers.Locale{Code: l.Code, IsSource: l.IsSource, File: l.File})
	}
	for _, m := range s.Messages {
		p.Messages = append(p.Messages, layers.Message{
			Key: m.Key, Namespace: m.Namespace, Revision: m.Revision, Model: m.Model,
			Invalid: invalid(m.Invalid), File: m.File,
		})
	}
	for locale, trs := range s.Translations {
		out := make(map[string]layers.Translation, len(trs))
		for key, t := range trs {
			out[key] = layers.Translation{
				Key: t.Key, Locale: t.Locale, Model: t.Model, State: t.State,
				SourceRevision: t.SourceRevision, Outdated: t.Outdated, Warnings: t.Warnings,
				Invalid: invalid(t.Invalid), File: t.File,
			}
		}
		p.Translations[locale] = out
	}
	return p
}

func invalid(i *snapshot.Invalid) *layers.Invalid {
	if i == nil {
		return nil
	}
	return &layers.Invalid{Code: i.Code, Detail: i.Detail}
}

// render flattens a quality run into the command's wire shape.
func render(r qualityapp.Report) Report {
	out := Report{
		Origin: r.Origin, Messages: r.Messages, Invalid: r.Invalid, Findings: []Finding{},
		Errors: r.Counts.Errors, Warnings: r.Counts.Warnings, Passed: r.Passed(),
	}
	for _, l := range r.Locales {
		out.Locales = append(out.Locales, LocaleReport{
			Code: l.Code, IsSource: l.IsSource, Required: l.Required, Messages: l.Messages,
			Translated: l.Translated, Missing: l.Missing, Outdated: l.Outdated,
			Errors: l.Errors, Warnings: l.Warnings, Complete: l.Complete,
		})
	}
	for _, f := range r.Findings {
		out.Findings = append(out.Findings, Finding{
			Check: CheckName(f.Layer), Code: f.Code, Severity: f.Severity,
			Locale: f.Locus.Locale, Key: f.Locus.Key, Subject: f.Subject, Detail: f.Detail,
			Message: f.Message, Where: f.Locus.File,
		})
	}
	return out
}

// CheckName is a layer's name in `glossa check`'s output.
//
// The parity layer is still spelled `arguments` there: that is what the
// command has printed since M1, and renaming it is part of rebuilding
// the command on the Quality library (RFC 0005 §13 wave 3), not of
// moving the layers.
func CheckName(l domain.Layer) string {
	if l == domain.LayerParity {
		return "arguments"
	}
	return string(l)
}
