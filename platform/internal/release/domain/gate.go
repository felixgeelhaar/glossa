package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
)

// The publish gate's errors (RFC 0005 §4.1).
var (
	// ErrPolicyNotMet is a publish into an environment whose check
	// policy the release does not satisfy. *PolicyNotMetError says what
	// it asked for.
	ErrPolicyNotMet = errors.New("release: the environment's check policy is not met")
	// ErrForceNeedsReason is a forced publish that says nothing about
	// why. The reason is mandatory for the same purpose waivers have
	// one: an override nobody has to justify is indistinguishable from
	// the check not existing.
	ErrForceNeedsReason = errors.New("release: a forced publish needs a reason")
	// ErrInvalidForceReason is a reason longer than MaxForceReasonLen.
	ErrInvalidForceReason = errors.New("release: a force reason is at most 1000 characters")
)

// MaxForceReasonLen bounds an override's reason.
const MaxForceReasonLen = 1000

// Override is a deployment that went out although the environment's
// check policy refused it, and the reason its author gave.
//
// It is recorded on the deployment rather than on the release: a
// release is immutable and every environment that ever serves it serves
// the same bytes, so "this went out against production's gate" is a
// fact about the pointer move, not about the text.
type Override struct {
	// Forced says the gate refused and the publish went ahead anyway.
	Forced bool
	// Reason is why, in the publisher's words. Never empty when Forced.
	Reason string
}

// NewOverride validates a forced publish's reason.
func NewOverride(reason string) (Override, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Override{}, ErrForceNeedsReason
	}
	if utf8.RuneCountInString(reason) > MaxForceReasonLen {
		return Override{}, ErrInvalidForceReason
	}
	return Override{Forced: true, Reason: reason}, nil
}

// PolicyGate is what a project's check-policy document asks of a
// publish into one environment (RFC 0005 §4.1): which locales must be
// complete there, and which review state its text must have reached.
//
// It is resolved once, from the document, and then answers about a
// build. The document itself lives in Catalog's project settings and
// reaches Release through its Source port.
type PolicyGate struct {
	// Environment is the environment the gate is for.
	Environment string
	// Bound says the document names this environment at all. A document
	// that does not gates nothing there: `require_complete` at the
	// document level is what CI grades a pull request by, and reading it
	// as a publish requirement everywhere would stop every project that
	// has an untranslated locale from publishing to development.
	Bound bool
	// RequireComplete are the locales that must be complete, or nil for
	// every locale the release ships. It is the environment block's when
	// the block names one and the document's when it does not — "I did
	// not say" is not "every locale".
	RequireComplete []string
	// RequireReview is the review state every translation the release
	// ships must have reached (checkpolicy.ReviewApproved), or "" where
	// the environment asks for none.
	RequireReview string
}

// NewPolicyGate resolves doc for environment.
func NewPolicyGate(doc checkpolicy.Policy, environment string) PolicyGate {
	block, ok := doc.Environments[environment]
	if !ok {
		return PolicyGate{Environment: environment}
	}
	// In resolves the block's require_complete into the document's base,
	// inheriting when the block named none: the one piece of code that
	// decides this, so that the publish gate and `glossa check` cannot
	// drift apart about what an environment requires.
	return PolicyGate{
		Environment:     environment,
		Bound:           true,
		RequireComplete: slices.Clone(doc.In(environment).RequireComplete),
		RequireReview:   block.RequireReview,
	}
}

// Unmet is one thing the gate asked for that the release does not meet.
type Unmet struct {
	// Locale is the required locale that is not complete, or "" when the
	// review state is what is missing.
	Locale string
	Detail string
}

// PolicyNotMetError lists everything the gate refused at once, so a
// publisher fixes all of it rather than discovering it one locale at a
// time. It is ErrPolicyNotMet.
type PolicyNotMetError struct {
	Environment string
	Unmet       []Unmet
}

func (e *PolicyNotMetError) Error() string {
	details := make([]string, 0, len(e.Unmet))
	for _, u := range e.Unmet {
		details = append(details, u.Detail)
	}
	return fmt.Sprintf("%s: %s: %s", ErrPolicyNotMet.Error(), e.Environment, strings.Join(details, "; "))
}

// Unwrap makes the error ErrPolicyNotMet.
func (e *PolicyNotMetError) Unwrap() error { return ErrPolicyNotMet }

// Check reports why a release built under shipped may not publish into
// the gate's environment, or nil. shipped is the environment's
// eligibility policy — which review states its releases carry.
func (g PolicyGate) Check(shipped Policy, b Built) error {
	if !g.Bound {
		return nil
	}
	var unmet []Unmet
	for _, locale := range g.required(b) {
		ls, ok := b.Stats.Locales[locale]
		switch {
		case !ok:
			unmet = append(unmet, Unmet{Locale: locale,
				Detail: fmt.Sprintf("%s must be complete and the release does not ship it", locale)})
		case ls.Messages < b.Stats.Messages:
			unmet = append(unmet, Unmet{Locale: locale,
				Detail: fmt.Sprintf("%s must be complete and is %d of %d messages short",
					locale, b.Stats.Messages-ls.Messages, b.Stats.Messages)})
		}
	}
	if g.RequireReview != "" && !shipped.Reached(g.RequireReview) {
		unmet = append(unmet, Unmet{Detail: fmt.Sprintf(
			"the environment requires %s text and its policy ships %s",
			g.RequireReview, strings.Join(shipped.States, ", "))})
	}
	if len(unmet) == 0 {
		return nil
	}
	return &PolicyNotMetError{Environment: g.Environment, Unmet: unmet}
}

// required lists the locales that must be complete, sorted, so a
// refusal reads the same every time.
func (g PolicyGate) required(b Built) []string {
	var out []string
	if g.RequireComplete != nil {
		out = slices.Clone(g.RequireComplete)
	} else {
		out = make([]string, 0, len(b.Content.Locales))
		for _, l := range b.Content.Locales {
			out = append(out, l.Code)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Reached reports whether every translation a release built under p may
// ship has reached state: a policy that ships drafts has not reached
// approved, however much of its text happens to be approved.
func (p Policy) Reached(state string) bool {
	want := slices.Index(shippable, state)
	if want < 0 {
		return false
	}
	for _, s := range p.States {
		if slices.Index(shippable, s) < want {
			return false
		}
	}
	return true
}
