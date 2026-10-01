package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

func mustProjects(t *testing.T, ids ...string) domain.ProjectScope {
	t.Helper()
	s, err := domain.ParseProjectScope(ids)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestParseProjectScope(t *testing.T) {
	a, b := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	s := mustProjects(t, b, a, strings.ToUpper(b))
	if want := []string{a, b}; !reflect.DeepEqual(s.Strings(), want) {
		t.Errorf("projects = %v, want %v (canonical, sorted, deduplicated)", s.Strings(), want)
	}
	if s.All() {
		t.Error("a scope naming projects is not every project")
	}
	ref, _ := domain.ParseProjectRef(a)
	other, _ := domain.ParseProjectRef(uuid.Must(uuid.NewV7()).String())
	if !s.Covers(ref) || s.Covers(other) {
		t.Error("a project scope covers exactly its projects")
	}
	if all := mustProjects(t); !all.All() || !all.Covers(other) {
		t.Error("the empty project scope is every project, as every membership is today")
	}
	if _, err := domain.ParseProjectScope([]string{"not-a-uuid"}); !errors.Is(err, domain.ErrInvalidID) {
		t.Errorf("malformed project err = %v", err)
	}
	many := make([]string, 101)
	for i := range many {
		many[i] = uuid.Must(uuid.NewV7()).String()
	}
	if _, err := domain.ParseProjectScope(many); !errors.Is(err, domain.ErrTooManyProjects) {
		t.Errorf("101 projects err = %v", err)
	}
}

func TestParseVisibility(t *testing.T) {
	for in, want := range map[string]domain.Visibility{"": domain.VisibilityAll, "all": domain.VisibilityAll, "assigned": domain.VisibilityAssigned} {
		if got, err := domain.ParseVisibility(in); err != nil || got != want {
			t.Errorf("ParseVisibility(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := domain.ParseVisibility("own"); !errors.Is(err, domain.ErrInvalidVisibility) {
		t.Errorf("unknown visibility err = %v", err)
	}
}

func TestInviteWithRestriction(t *testing.T) {
	tenant := tenancy.NewID()
	vendor := domain.NewVendorID()
	projects := mustProjects(t, uuid.Must(uuid.NewV7()).String())
	invite := func(roles []string, r domain.Restriction) (domain.Member, error) {
		return domain.InviteWith(tenant, tenancy.KindOrganization, mustEmail(t, "v@agency.example"),
			mustRoles(t, roles...), mustLocales(t, "de"), r, adminGrant(t), now)
	}

	m, err := invite([]string{"translator"}, domain.Restriction{Projects: projects, Vendor: vendor, Visibility: domain.VisibilityAssigned})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Restriction.Projects.Strings(), projects.Strings()) || m.Restriction.Vendor != vendor ||
		m.Restriction.Visibility != domain.VisibilityAssigned || !m.IsVendorMember() {
		t.Errorf("restriction = %+v", m.Restriction)
	}

	if _, err := invite([]string{"translator"}, domain.Restriction{Vendor: vendor}); !errors.Is(err, domain.ErrVendorMemberVisibility) {
		t.Errorf("a vendor member who sees the whole tenant err = %v", err)
	}
	// §3.3: an assigned member writes translations in their units and
	// can't review, publish, import or manage anything.
	for _, role := range []string{"reviewer", "developer", "admin", "owner"} {
		_, err := invite([]string{role}, domain.Restriction{Visibility: domain.VisibilityAssigned})
		if !errors.Is(err, domain.ErrAssignedVisibilityRole) && !errors.Is(err, domain.ErrLocalesNeedLocaleRole) {
			t.Errorf("assigned visibility with role %s err = %v", role, err)
		}
	}
	if _, err := domain.InviteWith(tenant, tenancy.KindOrganization, mustEmail(t, "o@example.com"), mustRoles(t, "owner"),
		domain.LocaleScope{}, domain.Restriction{Projects: projects}, ownerGrant(t), now); !errors.Is(err, domain.ErrOwnerProjectScoped) {
		t.Errorf("a project-scoped owner err = %v", err)
	}

	plain, err := domain.Invite(tenant, tenancy.KindOrganization, mustEmail(t, "p@example.com"), mustRoles(t, "translator"), domain.LocaleScope{}, adminGrant(t), now)
	if err != nil {
		t.Fatal(err)
	}
	if !plain.Restriction.Projects.All() || !plain.Restriction.Vendor.IsZero() || plain.Restriction.Visibility != domain.VisibilityAll {
		t.Errorf("an ordinary invitation is unrestricted, got %+v", plain.Restriction)
	}
	if o := domain.NewOwner(tenant, domain.NewPersonID(), mustEmail(t, "ada@example.com"), now); o.Restriction.Visibility != domain.VisibilityAll {
		t.Errorf("owner visibility = %q", o.Restriction.Visibility)
	}
}

func TestRestrict(t *testing.T) {
	m, err := domain.Invite(tenancy.NewID(), tenancy.KindOrganization, mustEmail(t, "tr@example.com"),
		mustRoles(t, "translator"), domain.LocaleScope{}, adminGrant(t), now)
	if err != nil {
		t.Fatal(err)
	}
	r := domain.Restriction{Projects: mustProjects(t, uuid.Must(uuid.NewV7()).String()), Visibility: domain.VisibilityAssigned}
	if err := m.Restrict(r, now.Add(1)); err != nil {
		t.Fatal(err)
	}
	if m.Version != 2 || m.Restriction.Visibility != domain.VisibilityAssigned {
		t.Errorf("restricted member = %+v", m)
	}
	// Roles and restriction are checked together, whichever changes.
	if err := m.ChangeAccess(mustRoles(t, "reviewer"), domain.LocaleScope{}, adminGrant(t), 1, now); !errors.Is(err, domain.ErrAssignedVisibilityRole) {
		t.Errorf("promoting an assigned member to reviewer err = %v", err)
	}
	if err := m.Restrict(domain.Restriction{Visibility: "own"}, now); !errors.Is(err, domain.ErrInvalidVisibility) {
		t.Errorf("unknown visibility err = %v", err)
	}
}

// THE GAP, PINNED. RFC 0006 §13 builds project scope and assignment
// visibility in two steps: wave 1 models them (this package stores and
// validates them), wave 2 enforces them in every read path at once,
// proved by the generated endpoint sweep of §12.2. Until wave 2 lands,
// a restricted member's grant is exactly an unrestricted member's, and
// no principal carries the restriction at all.
//
// This test fails the moment that changes, so whoever enforces the
// restriction also removes the NOT ENFORCED warnings on Restriction,
// Member.Restriction and APIToken.Projects — and nobody reads those
// fields believing they already protect anything.
func TestRestrictionIsModelledButNotYetEnforced(t *testing.T) {
	roles, locales := mustRoles(t, "translator"), mustLocales(t, "de")
	m, err := domain.InviteWith(tenancy.NewID(), tenancy.KindOrganization, mustEmail(t, "v@agency.example"), roles, locales,
		domain.Restriction{
			Projects: mustProjects(t, uuid.Must(uuid.NewV7()).String()), Vendor: domain.NewVendorID(),
			Visibility: domain.VisibilityAssigned,
		}, adminGrant(t), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Activate(domain.NewPersonID(), now); err != nil {
		t.Fatal(err)
	}
	unrestricted := domain.GrantForMember(roles, locales)
	if !reflect.DeepEqual(m.Grant().LocaleScopes(), unrestricted.LocaleScopes()) {
		t.Fatal("a restricted member's grant now differs from an unrestricted one: enforcement has begun. " +
			"Remove the NOT ENFORCED warnings (domain.Restriction, Member.Restriction, APIToken.Projects, authz's package doc) " +
			"and replace this test with the enforcement's own")
	}
	scopes, err := domain.ParseScopes([]string{"read", "write"})
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := domain.NewAPIToken(tenancy.NewID(), "ci", scopes, nil, domain.PersonActor(domain.NewPersonID()), now)
	if err != nil {
		t.Fatal(err)
	}
	tok.Projects = mustProjects(t, uuid.Must(uuid.NewV7()).String())
	if !reflect.DeepEqual(tok.Grant().LocaleScopes(), domain.GrantForScopes(scopes).LocaleScopes()) {
		t.Fatal("a project-scoped token's grant now differs from an unscoped one: enforcement has begun; replace this test")
	}
	if domain.RestrictionEnforced {
		t.Fatal("RestrictionEnforced is true: replace this test with the enforcement's own")
	}
}
