package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
)

// The vocabulary (RFC 0006 §2.4): a closed set of generic primitives the
// platform owns. A definition's chart names its guards and actions
// locally ("two_approvals", "assign_vendor"); each local name is bound
// to one primitive here with its parameters ("use": "approvals_at_least",
// "n": 2). Nothing in this file names an organisation, a team or a
// process; what a primitive means to an organisation lives in its
// definition.
//
// The vocabulary grows by RFC amendment, one primitive at a time, each
// with a reason that is not one organisation. vocabulary_test.go pins
// the exact list so that growing it is a deliberate act.

// EventName is an event that can move an instance.
type EventName string

// Events (§2.4). Localization's, Intelligence's and Quality's exist;
// assignment.*, approval.* and timer.* are Workflow's own.
const (
	EventTranslationRevised  EventName = "translation.revised"
	EventTranslationReviewed EventName = "translation.reviewed"
	EventTranslationOutdated EventName = "translation.outdated"
	EventSuggestionCreated   EventName = "suggestion.created"
	EventCheckRunRecorded    EventName = "check_run.recorded"
	EventAssignmentCompleted EventName = "assignment.completed"
	EventAssignmentDeclined  EventName = "assignment.declined"
	EventApprovalGranted     EventName = "approval.granted"
	EventApprovalDenied      EventName = "approval.denied"
	// EventTimerDue and EventTimerOverdue are raised by Workflow's sweep
	// on the kernel scheduler from a stored due_at — never by a timer in
	// the chart (§2.3, "no delayed transitions").
	EventTimerDue     EventName = "timer.due"
	EventTimerOverdue EventName = "timer.overdue"
)

var (
	translationOnly = []SubjectKind{SubjectTranslation}
	anySubject      = []SubjectKind{SubjectTranslation, SubjectReleaseRequest}
)

// events maps every event to the subjects it can concern.
var events = map[EventName][]SubjectKind{
	EventTranslationRevised:  translationOnly,
	EventTranslationReviewed: translationOnly,
	EventTranslationOutdated: translationOnly,
	EventSuggestionCreated:   translationOnly,
	EventCheckRunRecorded:    translationOnly,
	EventAssignmentCompleted: anySubject,
	EventAssignmentDeclined:  anySubject,
	EventApprovalGranted:     anySubject,
	EventApprovalDenied:      anySubject,
	EventTimerDue:            anySubject,
	EventTimerOverdue:        anySubject,
}

// Events lists every event name, sorted.
func Events() []EventName { return slices.Sorted(maps.Keys(events)) }

// GuardFunc is a compiled guard: a pure function of the step.
type GuardFunc func(Step) bool

type guardPrimitive struct {
	subjects []SubjectKind
	compile  func(params []byte) (GuardFunc, any, error)
}

type actionPrimitive struct {
	subjects []SubjectKind
	compile  func(params []byte) (any, error)
}

// GuardPrimitives lists every guard primitive, sorted.
func GuardPrimitives() []string { return slices.Sorted(maps.Keys(guardPrimitives)) }

// ActionPrimitives lists every action primitive, sorted.
func ActionPrimitives() []string { return slices.Sorted(maps.Keys(actionPrimitives)) }

// The vocabulary's mirrors of other contexts' values. Workflow's domain
// imports no other context; vocabulary_test.go proves each list agrees
// with the context that owns it, as quality/domain.Layers does with the
// kernel's.
var (
	// ReviewStates are Localization's four. A workflow can move a
	// translation between them; it can never add one (§2.1 rule 1).
	ReviewStates = []string{"draft", "needs_review", "approved", "rejected"}
	// Origins are Localization's translation provenances.
	Origins = []string{"human", "ai", "agent", "translation_memory", "machine_translation", "import", "adaptation"}
	// Bands are Intelligence's routing bands, lowest confidence first.
	Bands = []string{"review_required", "approve_recommended", "auto_approve"}
	// Roles are Identity's built-in roles.
	Roles = []string{"owner", "admin", "developer", "translator", "reviewer"}
)

// severities a findings guard counts, lowest first.
var severities = []string{string(checkpolicy.Warning), string(checkpolicy.Error)}

// Limits on a parameter (RFC 0006 §9.6 bounds the rest).
const (
	maxListParam  = 100
	maxApprovals  = 10
	maxNameLength = 64
)

var (
	permissionPattern = regexp.MustCompile(`^[a-z]+\.[a-z]+$`)
	refPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
)

// ErrParams is a primitive's parameters being wrong.
var ErrParams = errors.New("invalid parameters")

func paramErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrParams, fmt.Sprintf(format, a...))
}

// decodeStrict reads params into v, refusing a field v does not have:
// a parameter the platform would silently ignore is a mistake the
// author should hear about at save, not discover at run time.
func decodeStrict(params []byte, v any) error {
	if len(bytes.TrimSpace(params)) == 0 {
		params = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, ErrParams) {
			return err
		}
		return paramErr("%s", strings.TrimPrefix(err.Error(), "json: "))
	}
	return nil
}

// ── guards ──────────────────────────────────────────────────────────

// OriginIn holds when the current revision's origin is one of Origins.
type OriginIn struct {
	Origins []string `json:"origins"`
}

// ConfidenceAtLeast holds when the latest suggestion's band is Band or
// higher.
type ConfidenceAtLeast struct {
	Band string `json:"band"`
}

// FindingsAtLeast holds when the subject has at least Count open
// findings at Severity or worse, in Layer (any layer when empty).
type FindingsAtLeast struct {
	Layer    string `json:"layer,omitempty"`
	Severity string `json:"severity"`
	Count    int    `json:"count"`
}

// LocaleIn holds when the subject's locale is one of Locales.
type LocaleIn struct {
	Locales []string `json:"locales"`
}

// NamespaceIn holds when the subject's namespace is one of Namespaces.
type NamespaceIn struct {
	Namespaces []string `json:"namespaces"`
}

// ApprovalsAtLeast holds when at least N distinct people have approved,
// not counting the author when DistinctFromAuthor is set.
type ApprovalsAtLeast struct {
	N                  int  `json:"n"`
	DistinctFromAuthor bool `json:"distinct_from_author,omitempty"`
}

// ActorHasPermission holds when the actor who raised the event holds
// Permission for the subject.
type ActorHasPermission struct {
	Permission string `json:"permission"`
}

// TMMatchAtLeast holds when the best translation-memory match scores at
// least Score, in (0, 1].
type TMMatchAtLeast struct {
	Score float64 `json:"score"`
}

var guardPrimitives = map[string]guardPrimitive{
	"origin_in": {translationOnly, func(raw []byte) (GuardFunc, any, error) {
		var p OriginIn
		if err := decodeStrict(raw, &p); err != nil {
			return nil, nil, err
		}
		if err := oneOfEach("origins", p.Origins, Origins); err != nil {
			return nil, nil, err
		}
		return func(s Step) bool { return slices.Contains(p.Origins, s.Subject.Origin) }, p, nil
	}},
	"confidence_at_least": {translationOnly, func(raw []byte) (GuardFunc, any, error) {
		var p ConfidenceAtLeast
		if err := decodeStrict(raw, &p); err != nil {
			return nil, nil, err
		}
		min := slices.Index(Bands, p.Band)
		if min < 0 {
			return nil, nil, paramErr("band must be one of %s, not %q", strings.Join(Bands, ", "), p.Band)
		}
		return func(s Step) bool { return slices.Index(Bands, s.Subject.Band) >= min }, p, nil
	}},
	"findings_at_least": {translationOnly, func(raw []byte) (GuardFunc, any, error) {
		var p FindingsAtLeast
		if err := decodeStrict(raw, &p); err != nil {
			return nil, nil, err
		}
		if p.Layer != "" && !slices.Contains(checkpolicy.Layers, p.Layer) {
			return nil, nil, paramErr("layer must be one of %s, not %q", strings.Join(checkpolicy.Layers, ", "), p.Layer)
		}
		min := slices.Index(severities, p.Severity)
		if min < 0 {
			return nil, nil, paramErr("severity must be warning or error, not %q", p.Severity)
		}
		if p.Count < 1 {
			return nil, nil, paramErr("count must be at least 1")
		}
		return func(s Step) bool {
			n := 0
			for _, f := range s.Subject.Findings {
				if (p.Layer == "" || f.Layer == p.Layer) && slices.Index(severities, f.Severity) >= min {
					n += f.Count
				}
			}
			return n >= p.Count
		}, p, nil
	}},
	"locale_in": {translationOnly, func(raw []byte) (GuardFunc, any, error) {
		var p LocaleIn
		if err := decodeStrict(raw, &p); err != nil {
			return nil, nil, err
		}
		locales, err := CanonicalLocales(p.Locales)
		if err != nil {
			return nil, nil, err
		}
		if len(locales) == 0 {
			return nil, nil, paramErr("locales must name at least one locale")
		}
		p.Locales = locales
		return func(s Step) bool {
			tag, err := bcp47.Parse(s.Subject.Locale)
			return err == nil && slices.Contains(p.Locales, tag.String())
		}, p, nil
	}},
	"namespace_in": {translationOnly, func(raw []byte) (GuardFunc, any, error) {
		var p NamespaceIn
		if err := decodeStrict(raw, &p); err != nil {
			return nil, nil, err
		}
		if len(p.Namespaces) == 0 || len(p.Namespaces) > maxListParam {
			return nil, nil, paramErr("namespaces must name between 1 and %d namespaces", maxListParam)
		}
		for _, ns := range p.Namespaces {
			if strings.TrimSpace(ns) == "" {
				return nil, nil, paramErr("a namespace may not be empty")
			}
		}
		return func(s Step) bool { return slices.Contains(p.Namespaces, s.Subject.Namespace) }, p, nil
	}},
	"approvals_at_least": {anySubject, func(raw []byte) (GuardFunc, any, error) {
		var p ApprovalsAtLeast
		if err := decodeStrict(raw, &p); err != nil {
			return nil, nil, err
		}
		if p.N < 1 || p.N > maxApprovals {
			return nil, nil, paramErr("n must be between 1 and %d", maxApprovals)
		}
		return func(s Step) bool {
			seen := map[string]bool{}
			for _, a := range s.Subject.Approvers {
				if a == "" || (p.DistinctFromAuthor && a == s.Subject.Author) {
					continue
				}
				seen[a] = true
			}
			return len(seen) >= p.N
		}, p, nil
	}},
	"actor_has_permission": {anySubject, func(raw []byte) (GuardFunc, any, error) {
		var p ActorHasPermission
		if err := decodeStrict(raw, &p); err != nil {
			return nil, nil, err
		}
		if !permissionPattern.MatchString(p.Permission) {
			return nil, nil, paramErr("permission must be a permission name like translations.review, not %q", p.Permission)
		}
		return func(s Step) bool { return slices.Contains(s.Trigger.Permissions, p.Permission) }, p, nil
	}},
	"tm_match_at_least": {translationOnly, func(raw []byte) (GuardFunc, any, error) {
		var p TMMatchAtLeast
		if err := decodeStrict(raw, &p); err != nil {
			return nil, nil, err
		}
		if !(p.Score > 0 && p.Score <= 1) {
			return nil, nil, paramErr("score must be in (0, 1], not %v", p.Score)
		}
		return func(s Step) bool { return s.Subject.TMMatch >= p.Score }, p, nil
	}},
}

// ── actions ─────────────────────────────────────────────────────────

// Party names who an assignment, an approval request or a notification
// goes to: exactly one member, role, group or vendor. Groups and vendors
// are data the organisation chooses (RFC 0006 §4.3) — that is what keeps
// "legal" out of the code.
type Party struct {
	Member string `json:"member,omitempty"`
	Role   string `json:"role,omitempty"`
	Group  string `json:"group,omitempty"`
	Vendor string `json:"vendor,omitempty"`
}

// validate checks that p names exactly one party of a kind in allowed.
func (p Party) validate(field string, allowed ...string) error {
	named := map[string]string{"member": p.Member, "role": p.Role, "group": p.Group, "vendor": p.Vendor}
	var kinds []string
	for k, v := range named {
		if v != "" {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) != 1 {
		return paramErr("%s must name exactly one of %s", field, strings.Join(allowed, ", "))
	}
	kind := kinds[0]
	if !slices.Contains(allowed, kind) {
		return paramErr("%s may name %s, not a %s", field, strings.Join(allowed, ", "), kind)
	}
	if kind == "role" {
		if !slices.Contains(Roles, p.Role) {
			return paramErr("%s.role must be one of %s, not %q", field, strings.Join(Roles, ", "), p.Role)
		}
		return nil
	}
	if !refPattern.MatchString(named[kind]) {
		return paramErr("%s.%s must be an id or slug, not %q", field, kind, named[kind])
	}
	return nil
}

// Assign assigns the subject to a party, optionally due after Due.
type Assign struct {
	To  Party    `json:"to"`
	Due Duration `json:"due,omitzero"`
}

// RequestApproval asks N people from a party to approve the subject.
type RequestApproval struct {
	N    int      `json:"n"`
	From Party    `json:"from"`
	Due  Duration `json:"due,omitzero"`
}

// SetReviewState moves the translation to one of the four review
// states, through Localization's ReviewTranslation port as the
// triggering actor: Localization's own checks decide (§2.5).
type SetReviewState struct {
	State string `json:"state"`
}

// RequestFill asks Intelligence for a fill job for the subject. It
// takes no parameters in M5; §2.6 keeps room for an adaptation task.
type RequestFill struct{}

// RunCheck runs Quality's deterministic layers on the subject — all of
// them, or Layers.
type RunCheck struct {
	Layers []string `json:"layers,omitempty"`
}

// Notify tells a party in-app (and by mail when mail is configured).
type Notify struct {
	To Party `json:"to"`
}

var actionPrimitives = map[string]actionPrimitive{
	"assign": {anySubject, func(raw []byte) (any, error) {
		var p Assign
		if err := decodeStrict(raw, &p); err != nil {
			return nil, err
		}
		return p, p.To.validate("to", "member", "role", "group", "vendor")
	}},
	"request_approval": {anySubject, func(raw []byte) (any, error) {
		var p RequestApproval
		if err := decodeStrict(raw, &p); err != nil {
			return nil, err
		}
		if p.N < 1 || p.N > maxApprovals {
			return nil, paramErr("n must be between 1 and %d", maxApprovals)
		}
		// Approvals are human and come from inside the organisation's
		// own roles and groups (§9.3); a vendor delivers work, it does
		// not sign it off.
		return p, p.From.validate("from", "member", "role", "group")
	}},
	"set_review_state": {translationOnly, func(raw []byte) (any, error) {
		var p SetReviewState
		if err := decodeStrict(raw, &p); err != nil {
			return nil, err
		}
		if !slices.Contains(ReviewStates, p.State) {
			return nil, paramErr("state must be one of %s, not %q", strings.Join(ReviewStates, ", "), p.State)
		}
		return p, nil
	}},
	"request_fill": {translationOnly, func(raw []byte) (any, error) {
		var p RequestFill
		return p, decodeStrict(raw, &p)
	}},
	"run_check": {translationOnly, func(raw []byte) (any, error) {
		var p RunCheck
		if err := decodeStrict(raw, &p); err != nil {
			return nil, err
		}
		for _, l := range p.Layers {
			if !slices.Contains(checkpolicy.Layers, l) || checkpolicy.Advisory(l) {
				return nil, paramErr("layers must be deterministic Quality layers; %q is not one", l)
			}
		}
		return p, nil
	}},
	"notify": {anySubject, func(raw []byte) (any, error) {
		var p Notify
		if err := decodeStrict(raw, &p); err != nil {
			return nil, err
		}
		return p, p.To.validate("to", "member", "role", "group", "vendor")
	}},
}

// ── shared parameter checks ─────────────────────────────────────────

func oneOfEach(field string, got, allowed []string) error {
	if len(got) == 0 {
		return paramErr("%s must name at least one value", field)
	}
	for _, v := range got {
		if !slices.Contains(allowed, v) {
			return paramErr("%s must be among %s, not %q", field, strings.Join(allowed, ", "), v)
		}
	}
	return nil
}

// CanonicalLocales parses tags as BCP 47, and returns them canonical,
// sorted and without duplicates.
func CanonicalLocales(tags []string) ([]string, error) {
	if len(tags) > maxListParam {
		return nil, paramErr("at most %d locales", maxListParam)
	}
	out := make([]string, 0, len(tags))
	for _, s := range tags {
		tag, err := bcp47.Parse(s)
		if err != nil {
			return nil, paramErr("%q is not a BCP 47 locale", s)
		}
		out = append(out, tag.String())
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
