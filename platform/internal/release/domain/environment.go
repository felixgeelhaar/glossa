// Package domain is the Release context's model: what ships where
// (RFC 0002 §4, §7; intent §34–38). An Environment points at a Release;
// a Release is an immutable snapshot of a project's releasable text,
// written as content-addressed artifacts and described by a signed
// manifest (runtimes/SPEC.md §1). Publishing builds a release; promote
// and rollback only move environment pointers.
package domain

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/release/delivery"
)

// Errors.
var (
	ErrInvalidEnvironment = errors.New("release: environment names are 1-63 characters of [a-z0-9-], starting with a letter or digit (\"a\" is reserved)")
	ErrInvalidPolicy      = errors.New("release: a policy ships one or more of draft, needs_review and approved (never rejected)")
	ErrInvalidNote        = errors.New("release: a note is at most 1000 characters")
	ErrInvalidKeyName     = errors.New("release: a delivery key name is 1-200 characters")
	ErrNotReleasable      = errors.New("release: the catalog can't be released")
	ErrIneligible         = errors.New("release: the release ships review states this environment's policy excludes")
	ErrNoRollbackTarget   = errors.New("release: the environment has no earlier release to roll back to")
	ErrNotInHistory       = errors.New("release: the environment never served that release")
	ErrKeyRevoked         = errors.New("release: the delivery key is already revoked")
)

// The default environments every project has (intent §37).
const (
	Development = "development"
	Preview     = "preview"
	Staging     = "staging"
	Production  = "production"
)

// DefaultEnvironments are created with a project's first use of Release.
var DefaultEnvironments = []string{Development, Preview, Staging, Production}

// ParseEnvironmentName validates name.
func ParseEnvironmentName(name string) (string, error) {
	if !delivery.ValidEnvironment(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidEnvironment, name)
	}
	return name, nil
}

// Review states a policy may ship, in workflow order. Rejected text
// never ships.
const (
	StateDraft       = "draft"
	StateNeedsReview = "needs_review"
	StateApproved    = "approved"
)

var shippable = []string{StateDraft, StateNeedsReview, StateApproved}

// Policy is an environment's eligibility rule: which review states of a
// translation ship there, and whether a translation made against an
// older source revision (outdated) still ships. A locale without an
// eligible translation falls back at runtime (SPEC §4); it is never
// padded with source text.
type Policy struct {
	States          []string `json:"states"`
	IncludeOutdated bool     `json:"include_outdated"`
}

// NewPolicy validates states and returns them in workflow order.
func NewPolicy(states []string, includeOutdated bool) (Policy, error) {
	if len(states) == 0 {
		return Policy{}, ErrInvalidPolicy
	}
	var out []string
	for _, s := range shippable {
		if slices.Contains(states, s) {
			out = append(out, s)
		}
	}
	for _, s := range states {
		if !slices.Contains(shippable, s) {
			return Policy{}, fmt.Errorf("%w: %q", ErrInvalidPolicy, s)
		}
	}
	return Policy{States: out, IncludeOutdated: includeOutdated}, nil
}

// DefaultPolicy is the policy a default environment starts with:
// production and staging ship approved text only; development, preview
// and custom environments everything not rejected. Outdated text ships
// everywhere until an editor says otherwise: an approved translation of
// an older source is closer to right than a fallback language.
func DefaultPolicy(environment string) Policy {
	if environment == Production || environment == Staging {
		return Policy{States: []string{StateApproved}, IncludeOutdated: true}
	}
	return Policy{States: slices.Clone(shippable), IncludeOutdated: true}
}

// Covers reports whether everything a release built under other may ship
// is allowed under p: promoting a preview release with drafts into
// production must not smuggle drafts in.
func (p Policy) Covers(other Policy) bool {
	for _, s := range other.States {
		if !slices.Contains(p.States, s) {
			return false
		}
	}
	return p.IncludeOutdated || !other.IncludeOutdated
}

// Equal reports whether p and o are the same policy.
func (p Policy) Equal(o Policy) bool {
	return slices.Equal(p.States, o.States) && p.IncludeOutdated == o.IncludeOutdated
}

// Environment is where a project's text is served (development,
// production, a branch preview): a policy for what may ship there and a
// pointer to the release it serves.
type Environment struct {
	ProjectID uuid.UUID
	Name      string
	Policy    Policy
	// Current is the release served, uuid.Nil before the first publish.
	Current uuid.UUID
	// Version increments with every change (policy or pointer); it is
	// the environment's ETag.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewEnvironment creates an environment with policy.
func NewEnvironment(project uuid.UUID, name string, policy Policy, now time.Time) (Environment, error) {
	if _, err := ParseEnvironmentName(name); err != nil {
		return Environment{}, err
	}
	if _, err := NewPolicy(policy.States, policy.IncludeOutdated); err != nil {
		return Environment{}, err
	}
	return Environment{ProjectID: project, Name: name, Policy: policy, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

// ChangePolicy replaces the policy; false if it is unchanged. The
// release served keeps serving: a policy decides what the next publish
// takes.
func (e *Environment) ChangePolicy(p Policy, now time.Time) bool {
	if e.Policy.Equal(p) {
		return false
	}
	e.Policy = p
	e.touch(now)
	return true
}

// Point makes the environment serve release; false if it already does.
func (e *Environment) Point(release uuid.UUID, now time.Time) bool {
	if e.Current == release {
		return false
	}
	e.Current = release
	e.touch(now)
	return true
}

func (e *Environment) touch(now time.Time) {
	e.Version++
	e.UpdatedAt = now
}

// Action is how an environment came to serve a release.
type Action string

// Actions.
const (
	ActionPublish  Action = "publish"
	ActionPromote  Action = "promote"
	ActionRollback Action = "rollback"
)

// Deployment is one pointer move, kept forever: the environment's
// history, which rollback walks back through.
type Deployment struct {
	ProjectID   uuid.UUID
	Environment string
	// Number counts the environment's deployments from 1.
	Number    int
	ReleaseID uuid.UUID
	// Previous is what the environment served before (uuid.Nil if
	// nothing).
	Previous  uuid.UUID
	Action    Action
	By        string
	CreatedAt time.Time
}
