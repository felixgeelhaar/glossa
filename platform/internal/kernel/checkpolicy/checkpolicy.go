// Package checkpolicy is a project's check policy: which locales must
// be complete, which findings fail the check, and whether an
// untranslated key is one of them.
//
// It lives in the kernel because two callers decide the same question
// and must decide it the same way: `glossa check` in the CLI (where the
// policy is read from `glossa.yaml` as `require_complete` and `fail_on`)
// and the Glossa PR check on the server, which reports "per the
// project's check policy (the same `require_complete` and `fail_on` as
// `glossa check`)" (RFC 0004 §6.4). A second, subtly different policy
// would make the PR check disagree with the command developers run
// locally, which is the one thing a check must never do.
//
// The project stores one (Catalog's `settings.check_policy`), so the
// two agree by construction rather than by both defaulting to the same
// thing: the PR check reads the stored policy, and `glossa check` reads
// it too whenever it can reach the server.
//
// The codes are shared for the same reason: a finding a person saw as
// `missing-translation` on their terminal is the same finding in the
// pull request.
package checkpolicy

import (
	"errors"
	"fmt"
	"slices"
)

// Severity ranks a finding.
type Severity string

// Severities. Never ranks no finding: it is a FailOn value alone,
// meaning nothing the check finds fails it.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Never   Severity = "never"
)

// Policy errors.
var (
	// ErrInvalidSeverity is a fail_on or missing_translations that names
	// no severity.
	ErrInvalidSeverity = errors.New("checkpolicy: invalid severity")
	// ErrUnknownLocale is a required locale the project does not have.
	ErrUnknownLocale = errors.New("checkpolicy: the project has no such locale")
)

// Finding codes Glossa adds to the MessageFormat kernel's (compat
// findings keep the kernel's codes, e.g. missing-argument).
const (
	CodeInvalidMessage      = "invalid-message"
	CodeInvalidTranslation  = "invalid-translation"
	CodeMissingTranslation  = "missing-translation"
	CodeOutdatedTranslation = "outdated-translation"
	CodeUnknownKey          = "unknown-key"
	CodeMissingLocale       = "missing-locale"
	// CodeKeyConflict is two open branches proposing the same new key
	// with different source (RFC 0004 §4.1). Only the PR check can see
	// it, because only the server knows the other branches.
	CodeKeyConflict = "key-conflict"
)

// Policy decides what fails a check. Its zero value is the documented
// default: every locale must be complete, an untranslated key in one is
// an error, and errors fail.
type Policy struct {
	// RequireComplete lists the locales whose missing translations are
	// errors; nil means every locale, and an empty slice means none.
	// Others' are warnings.
	RequireComplete []string `json:"require_complete"`
	// FailOn is the lowest severity that fails the check: Error (the
	// default, and what "" means), Warning, or Never — nothing fails it,
	// and the check only reports.
	FailOn Severity `json:"fail_on,omitempty"`
	// MissingTranslations is the severity a key with no translation gets
	// in a locale RequireComplete names: Error (the default, and what ""
	// means) or Warning. A locale it does not name warns either way.
	//
	// It is the one knob that is not about *reporting* the gap but about
	// whether the gap blocks: a team that translates after merging sets
	// it to Warning and keeps every locale required, so the check still
	// lists what is untranslated without failing the pull request.
	MissingTranslations Severity `json:"missing_translations,omitempty"`
}

// Requires reports whether locale must be complete.
func (p Policy) Requires(locale string) bool {
	if p.RequireComplete == nil {
		return true
	}
	return slices.Contains(p.RequireComplete, locale)
}

// Fails reports whether a finding of severity s fails the check.
func (p Policy) Fails(s Severity) bool {
	switch p.FailOn {
	case Never:
		return false
	case Warning:
		return s == Error || s == Warning
	default:
		return s == Error
	}
}

// Severity of a missing translation in locale: an error where the
// policy requires the locale to be complete and missing translations
// are errors, a warning elsewhere.
func (p Policy) Severity(locale string) Severity {
	if p.Requires(locale) && p.MissingTranslations != Warning {
		return Error
	}
	return Warning
}

// Equal reports whether p and o decide every question the same way.
func (p Policy) Equal(o Policy) bool {
	return slices.Equal(p.RequireComplete, o.RequireComplete) &&
		p.FailOn == o.FailOn && p.MissingTranslations == o.MissingTranslations
}

// ParseFailOn validates a fail_on: error, warning or never. "" is
// Error, the default.
func ParseFailOn(s string) (Severity, error) {
	switch Severity(s) {
	case "", Error:
		return Error, nil
	case Warning:
		return Warning, nil
	case Never:
		return Never, nil
	}
	return "", fmt.Errorf("%w: fail_on is error, warning or never, not %q", ErrInvalidSeverity, s)
}

// ParseFindingSeverity validates the severity a finding may carry:
// error or warning. "" is Error, the default.
func ParseFindingSeverity(s string) (Severity, error) {
	switch Severity(s) {
	case "", Error:
		return Error, nil
	case Warning:
		return Warning, nil
	}
	return "", fmt.Errorf("%w: missing_translations is error or warning, not %q", ErrInvalidSeverity, s)
}

// Validate checks the policy and returns it normalized: the severities
// spelled out, and a RequireComplete that names no locale left as the
// empty slice ("none") rather than as a slice of nothing in particular.
//
// known is the project's locales, the source locale included; every
// required locale must be one of them. A nil known skips that check,
// for a caller that does not know the project's locales (the CLI's
// flags, which the check itself reports as missing-locale).
func (p Policy) Validate(known []string) (Policy, error) {
	failOn, err := ParseFailOn(string(p.FailOn))
	if err != nil {
		return Policy{}, err
	}
	missing, err := ParseFindingSeverity(string(p.MissingTranslations))
	if err != nil {
		return Policy{}, err
	}
	out := Policy{RequireComplete: p.RequireComplete, FailOn: failOn, MissingTranslations: missing}
	if out.RequireComplete == nil {
		return out, nil
	}
	if known != nil {
		for _, l := range out.RequireComplete {
			if !slices.Contains(known, l) {
				return Policy{}, fmt.Errorf("%w: %q", ErrUnknownLocale, l)
			}
		}
	}
	if len(out.RequireComplete) == 0 {
		// An empty list is "no locale has to be complete". Keep it
		// non-nil and canonical, because nil means the opposite.
		out.RequireComplete = []string{}
	}
	return out, nil
}
