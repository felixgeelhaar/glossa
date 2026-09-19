package domain_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
)

func mustRoles(t *testing.T, rs ...string) domain.Roles {
	t.Helper()
	r, err := domain.ParseRoles(rs)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mustLocales(t *testing.T, ls ...string) domain.LocaleScope {
	t.Helper()
	s, err := domain.ParseLocaleScope(ls)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustLocale(t *testing.T, s string) domain.Locale {
	t.Helper()
	l, err := domain.ParseLocale(s)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestParseRoles(t *testing.T) {
	r, err := domain.ParseRoles([]string{"reviewer", "translator", "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"reviewer", "translator"}; !reflect.DeepEqual(r.Strings(), want) {
		t.Errorf("roles = %v, want %v (sorted, deduplicated)", r.Strings(), want)
	}
	if _, err := domain.ParseRoles([]string{"superuser"}); !errors.Is(err, domain.ErrInvalidRole) {
		t.Errorf("unknown role err = %v", err)
	}
	if _, err := domain.ParseRoles(nil); !errors.Is(err, domain.ErrNoRoles) {
		t.Errorf("empty roles err = %v", err)
	}
}

// The role matrix is the authorization contract other contexts rely on.
// Changing a row here is a product decision, not a refactor.
func TestRolePermissionMatrix(t *testing.T) {
	type perms = []domain.Permission
	all := domain.AllPermissions()
	tests := map[string]struct {
		allowed perms
	}{
		"owner": {allowed: all},
		"admin": {allowed: without(all, domain.PermOwnersManage)},
		"developer": {allowed: perms{
			domain.PermTenantRead, domain.PermMembersRead, domain.PermTokensRead, domain.PermTokensManage,
			domain.PermCatalogRead, domain.PermCatalogWrite, domain.PermTranslationsRead,
			domain.PermTranslationsWrite, domain.PermReleasesRead, domain.PermReleasesPublish,
		}},
		"translator": {allowed: perms{
			domain.PermTenantRead, domain.PermMembersRead, domain.PermCatalogRead,
			domain.PermTranslationsRead, domain.PermTranslationsWrite, domain.PermReleasesRead,
		}},
		"reviewer": {allowed: perms{
			domain.PermTenantRead, domain.PermMembersRead, domain.PermCatalogRead,
			domain.PermTranslationsRead, domain.PermTranslationsWrite, domain.PermTranslationsReview,
			domain.PermReleasesRead,
		}},
	}
	for role, tc := range tests {
		t.Run(role, func(t *testing.T) {
			g := domain.GrantForMember(mustRoles(t, role), domain.LocaleScope{})
			for _, p := range all {
				want := contains(tc.allowed, p)
				if got := g.Allows(p); got != want {
					t.Errorf("%s allows %s = %t, want %t", role, p, got, want)
				}
			}
		})
	}
}

func TestLocaleScopeRestrictsOnlyLocaleScopedPermissions(t *testing.T) {
	g := domain.GrantForMember(mustRoles(t, "translator"), mustLocales(t, "de", "fr"))
	de, deAT, ja := mustLocale(t, "de"), mustLocale(t, "de-AT"), mustLocale(t, "ja")

	if g.Allows(domain.PermTranslationsWrite) {
		t.Error("a locale-scoped translator must not write every locale")
	}
	if !g.AllowsFor(domain.PermTranslationsWrite, de) || !g.AllowsFor(domain.PermTranslationsWrite, deAT) {
		t.Error("translator scoped to de must write de and de-AT")
	}
	if g.AllowsFor(domain.PermTranslationsWrite, ja) {
		t.Error("translator scoped to de/fr must not write ja")
	}
	if !g.Allows(domain.PermCatalogRead) || !g.AllowsFor(domain.PermCatalogRead, ja) {
		t.Error("the locale scope must not restrict catalog.read")
	}
	if g.AllowsFor(domain.PermTranslationsReview, de) {
		t.Error("a translator must not review")
	}
}

func TestUnscopedRoleWinsOverScopedRole(t *testing.T) {
	// An admin who is also a locale-scoped reviewer reviews everything.
	g := domain.GrantForMember(mustRoles(t, "admin", "reviewer"), mustLocales(t, "de"))
	if !g.AllowsFor(domain.PermTranslationsReview, mustLocale(t, "ja")) {
		t.Error("admin must review every locale regardless of a reviewer locale scope")
	}
}

func TestParseScopes(t *testing.T) {
	s, err := domain.ParseScopes([]string{"write", "read", "write"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"read", "write"}; !reflect.DeepEqual(s.Strings(), want) {
		t.Errorf("scopes = %v, want %v", s.Strings(), want)
	}
	if _, err := domain.ParseScopes([]string{"root"}); !errors.Is(err, domain.ErrInvalidScope) {
		t.Errorf("unknown scope err = %v", err)
	}
	if _, err := domain.ParseScopes(nil); !errors.Is(err, domain.ErrInvalidScope) {
		t.Errorf("empty scopes err = %v", err)
	}
}

func TestScopeGrants(t *testing.T) {
	grant := func(scopes ...string) domain.Grant {
		s, err := domain.ParseScopes(scopes)
		if err != nil {
			t.Fatal(err)
		}
		return domain.GrantForScopes(s)
	}
	read := grant("read")
	if !read.Allows(domain.PermCatalogRead) || read.Allows(domain.PermCatalogWrite) {
		t.Error("read must read and not write")
	}
	write := grant("write")
	if !write.Allows(domain.PermCatalogRead) || !write.Allows(domain.PermTranslationsWrite) || write.Allows(domain.PermReleasesPublish) {
		t.Error("write implies read, grants writes, and does not publish")
	}
	if write.Allows(domain.PermTranslationsReview) {
		t.Error("review is a human decision; no token scope grants it")
	}
	publish := grant("publish")
	if !publish.Allows(domain.PermReleasesPublish) || publish.Allows(domain.PermCatalogWrite) {
		t.Error("publish publishes and does not write")
	}
	admin := grant("admin")
	if !admin.Allows(domain.PermMembersManage) || !admin.Allows(domain.PermTokensManage) {
		t.Error("admin manages members and tokens")
	}
	if admin.Allows(domain.PermOwnersManage) {
		t.Error("no token may manage owners")
	}
}

func TestGrantCovers(t *testing.T) {
	scopes := func(s ...string) domain.Grant {
		sc, err := domain.ParseScopes(s)
		if err != nil {
			t.Fatal(err)
		}
		return domain.GrantForScopes(sc)
	}
	developer := domain.GrantForMember(mustRoles(t, "developer"), domain.LocaleScope{})
	if !developer.Covers(scopes("read", "write", "publish")) {
		t.Error("a developer may mint a CI token: read, write, publish")
	}
	if developer.Covers(scopes("admin")) {
		t.Error("a developer must not mint an admin token")
	}
	scopedTranslator := domain.GrantForMember(mustRoles(t, "translator"), mustLocales(t, "de"))
	if scopedTranslator.Covers(scopes("write")) {
		t.Error("a locale-scoped grant must not cover an unscoped token")
	}
	owner := domain.GrantForMember(mustRoles(t, "owner"), domain.LocaleScope{})
	if !owner.Covers(scopes("read", "write", "publish", "admin")) {
		t.Error("an owner may mint any token")
	}
}

func without(ps []domain.Permission, drop domain.Permission) []domain.Permission {
	var out []domain.Permission
	for _, p := range ps {
		if p != drop {
			out = append(out, p)
		}
	}
	return out
}

func contains(ps []domain.Permission, p domain.Permission) bool {
	for _, q := range ps {
		if q == p {
			return true
		}
	}
	return false
}
