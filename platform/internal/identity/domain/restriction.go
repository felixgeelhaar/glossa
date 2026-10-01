package domain

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// RestrictionEnforced is false: project scope and assignment visibility
// are modelled, validated and stored, and NOTHING ENFORCES THEM YET.
//
// RFC 0006 §13 splits the work: wave 1 (this) adds the model; wave 2
// makes every read path in every context honour it at once, proved by a
// sweep over every endpoint generated from the OpenAPI document (§12.2).
// Enforcing it in a few paths first would only give a false sense of
// safety, so until wave 2:
//
//   - a project-scoped member or token can still reach every project;
//   - a member with VisibilityAssigned (every vendor member) can still
//     read everything their roles allow across the whole tenant;
//   - no authz.Principal carries a Restriction, and the system-scope
//     membership and token lookups that build principals cannot read the
//     columns that store one.
//
// Do not give a vendor access to a tenant on the strength of these
// fields. Wave 2 flips this constant together with the enforcement, and
// TestRestrictionIsModelledButNotYetEnforced fails until it does both.
const RestrictionEnforced = false

// maxProjectScope bounds a member's or token's project list.
const maxProjectScope = 100

// ProjectScope is the projects a member's roles, or a token's scopes,
// apply to (RFC 0006 §4.1). The zero value is every project — which is
// every membership and token that exists today.
//
// NOT ENFORCED until RFC 0006 wave 2: see RestrictionEnforced.
type ProjectScope struct{ projects []ProjectRef }

// ParseProjectScope canonicalizes, sorts and de-duplicates project ids.
// An empty list is every project.
func ParseProjectScope(ids []string) (ProjectScope, error) {
	var out []ProjectRef
	for _, s := range ids {
		p, err := ParseProjectRef(s)
		if err != nil {
			return ProjectScope{}, err
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	if len(out) > maxProjectScope {
		return ProjectScope{}, fmt.Errorf("%w: %d (at most %d)", ErrTooManyProjects, len(out), maxProjectScope)
	}
	slices.SortFunc(out, func(a, b ProjectRef) int { return strings.Compare(a.String(), b.String()) })
	return ProjectScope{projects: out}, nil
}

// All reports whether the scope is every project.
func (s ProjectScope) All() bool { return len(s.projects) == 0 }

// Covers reports whether the scope includes project.
func (s ProjectScope) Covers(project ProjectRef) bool {
	return s.All() || slices.Contains(s.projects, project)
}

// Strings returns the project ids, sorted; empty for every project.
func (s ProjectScope) Strings() []string {
	out := make([]string, len(s.projects))
	for i, p := range s.projects {
		out[i] = p.String()
	}
	return out
}

// UUIDs returns the project ids, sorted; empty for every project.
func (s ProjectScope) UUIDs() []uuid.UUID {
	out := make([]uuid.UUID, len(s.projects))
	for i, p := range s.projects {
		out[i] = p.UUID()
	}
	return out
}

// Visibility says how much of the tenant a member reads (RFC 0006 §3.3).
type Visibility string

const (
	// VisibilityAll reads whatever the member's roles allow: every
	// member today.
	VisibilityAll Visibility = "all"
	// VisibilityAssigned reads only the translation units of the
	// member's assignments and what translating them needs. Every vendor
	// member has it. NOT ENFORCED until RFC 0006 wave 2.
	VisibilityAssigned Visibility = "assigned"
)

// ParseVisibility validates a visibility; empty means VisibilityAll.
func ParseVisibility(s string) (Visibility, error) {
	switch v := Visibility(s); v {
	case "":
		return VisibilityAll, nil
	case VisibilityAll, VisibilityAssigned:
		return v, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidVisibility, s)
	}
}

// Restriction narrows a member beyond roles and locales: the projects
// their roles apply to (§4.1), the vendor they work for, and whether
// they read the tenant or only their assignments (§3.3). The zero value
// is unrestricted.
//
// NOT ENFORCED until RFC 0006 wave 2: see RestrictionEnforced. It is
// stored and validated; no read path, no authz check and no principal
// consults it yet.
type Restriction struct {
	Projects ProjectScope
	// Vendor is zero unless the member works for a vendor.
	Vendor     VendorID
	Visibility Visibility
}

// normalized fills the zero visibility in.
func (r Restriction) normalized() Restriction {
	if r.Visibility == "" {
		r.Visibility = VisibilityAll
	}
	return r
}

// check validates r against the roles it narrows.
//
// A vendor member reads only their assignments (§3.3), and someone who
// reads only their assignments writes translations in them and does
// nothing else — they "can't review, publish, export, import, or list
// anything else" — so the only role that fits is translator. An owner
// answers for the whole tenant, so an owner is never project-scoped.
func (r Restriction) check(roles Roles) error {
	if _, err := ParseVisibility(string(r.Visibility)); err != nil {
		return err
	}
	if !r.Vendor.IsZero() && r.Visibility != VisibilityAssigned {
		return ErrVendorMemberVisibility
	}
	if r.Visibility == VisibilityAssigned && !slices.Equal(roles, Roles{RoleTranslator}) {
		return ErrAssignedVisibilityRole
	}
	if !r.Projects.All() && roles.Has(RoleOwner) {
		return ErrOwnerProjectScoped
	}
	return nil
}

// VendorID identifies a vendor.
type VendorID uuid.UUID

// GroupID identifies a group.
type GroupID uuid.UUID

// NewVendorID returns a fresh, time-ordered ID.
func NewVendorID() VendorID { return VendorID(newV7()) }

// NewGroupID returns a fresh, time-ordered ID.
func NewGroupID() GroupID { return GroupID(newV7()) }

// ParseVendorID parses the canonical string form.
func ParseVendorID(s string) (VendorID, error) {
	u, err := parseID(s)
	return VendorID(u), err
}

// ParseGroupID parses the canonical string form.
func ParseGroupID(s string) (GroupID, error) {
	u, err := parseID(s)
	return GroupID(u), err
}

func (id VendorID) String() string  { return uuid.UUID(id).String() }
func (id VendorID) UUID() uuid.UUID { return uuid.UUID(id) }
func (id VendorID) IsZero() bool    { return uuid.UUID(id) == uuid.Nil }
func (id GroupID) String() string   { return uuid.UUID(id).String() }
func (id GroupID) UUID() uuid.UUID  { return uuid.UUID(id) }
func (id GroupID) IsZero() bool     { return uuid.UUID(id) == uuid.Nil }
