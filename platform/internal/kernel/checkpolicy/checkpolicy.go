// Package checkpolicy is a project's check policy: which locales must
// be complete, and which findings fail the check.
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
// The codes are shared for the same reason: a finding a person saw as
// `missing-translation` on their terminal is the same finding in the
// pull request.
package checkpolicy

// Severity ranks a finding.
type Severity string

// Severities.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
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

// Policy decides what fails a check.
type Policy struct {
	// RequireComplete lists the locales whose missing translations are
	// errors; nil means every locale. Others' are warnings.
	RequireComplete []string
	// FailOn is the lowest severity that fails the check (default Error).
	FailOn Severity
}

// Requires reports whether locale must be complete.
func (p Policy) Requires(locale string) bool {
	if p.RequireComplete == nil {
		return true
	}
	for _, l := range p.RequireComplete {
		if l == locale {
			return true
		}
	}
	return false
}

// Fails reports whether a finding of severity s fails the check.
func (p Policy) Fails(s Severity) bool {
	return s == Error || (p.FailOn == Warning && s == Warning)
}

// Severity of a missing translation in locale: an error where the
// policy requires the locale to be complete, a warning elsewhere.
func (p Policy) Severity(locale string) Severity {
	if p.Requires(locale) {
		return Error
	}
	return Warning
}
