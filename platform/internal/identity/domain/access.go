package domain

import (
	"fmt"
	"slices"
)

// Permission is one thing a principal may do in a tenant. Other bounded
// contexts check them through package authz; they are named
// "<resource>.<action>" and never renamed once released.
type Permission string

// The permissions. Locale-scoped ones (see LocaleScoped) can be limited
// to some locales for translators and reviewers.
const (
	PermTenantRead         Permission = "tenant.read"
	PermTenantManage       Permission = "tenant.manage"
	PermMembersRead        Permission = "members.read"
	PermMembersManage      Permission = "members.manage"
	PermOwnersManage       Permission = "owners.manage"
	PermTokensRead         Permission = "tokens.read"
	PermTokensManage       Permission = "tokens.manage"
	PermCatalogRead        Permission = "catalog.read"
	PermCatalogWrite       Permission = "catalog.write"
	PermKnowledgeRead      Permission = "knowledge.read"
	PermKnowledgeWrite     Permission = "knowledge.write"
	PermTranslationsRead   Permission = "translations.read"
	PermTranslationsWrite  Permission = "translations.write"
	PermTranslationsReview Permission = "translations.review"
	PermReleasesRead       Permission = "releases.read"
	PermReleasesPublish    Permission = "releases.publish"
)

// AllPermissions lists every permission, sorted.
func AllPermissions() []Permission {
	return []Permission{
		PermCatalogRead, PermCatalogWrite,
		PermKnowledgeRead, PermKnowledgeWrite,
		PermMembersManage, PermMembersRead,
		PermOwnersManage,
		PermReleasesPublish, PermReleasesRead,
		PermTenantManage, PermTenantRead,
		PermTokensManage, PermTokensRead,
		PermTranslationsRead, PermTranslationsReview, PermTranslationsWrite,
	}
}

// LocaleScoped reports whether a member's locale scope limits p.
func (p Permission) LocaleScoped() bool {
	return p == PermTranslationsWrite || p == PermTranslationsReview
}

// Role is a named bundle of permissions a member holds in a tenant.
type Role string

// The roles.
const (
	RoleOwner      Role = "owner"
	RoleAdmin      Role = "admin"
	RoleDeveloper  Role = "developer"
	RoleTranslator Role = "translator"
	RoleReviewer   Role = "reviewer"
)

var readAll = []Permission{
	PermTenantRead, PermMembersRead, PermCatalogRead, PermTranslationsRead, PermReleasesRead, PermKnowledgeRead,
}

// rolePermissions is the role matrix; TestRolePermissionMatrix pins it.
var rolePermissions = map[Role][]Permission{
	RoleOwner: AllPermissions(),
	RoleAdmin: slices.DeleteFunc(AllPermissions(), func(p Permission) bool { return p == PermOwnersManage }),
	RoleDeveloper: append(slices.Clone(readAll),
		PermTokensRead, PermTokensManage, PermCatalogWrite, PermTranslationsWrite, PermReleasesPublish,
		PermKnowledgeWrite),
	RoleTranslator: append(slices.Clone(readAll), PermTranslationsWrite),
	RoleReviewer:   append(slices.Clone(readAll), PermTranslationsWrite, PermTranslationsReview),
}

// localeRoles are the roles a locale scope applies to.
func (r Role) localeScoped() bool { return r == RoleTranslator || r == RoleReviewer }

// ParseRole validates a role name.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if _, ok := rolePermissions[r]; !ok {
		return "", fmt.Errorf("%w: %q", ErrInvalidRole, s)
	}
	return r, nil
}

// Roles is a non-empty, sorted, duplicate-free set of roles.
type Roles []Role

// ParseRoles validates, sorts and de-duplicates role names.
func ParseRoles(names []string) (Roles, error) {
	var out Roles
	for _, n := range names {
		r, err := ParseRole(n)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil, ErrNoRoles
	}
	slices.Sort(out)
	return out, nil
}

// Has reports whether r is in the set.
func (rs Roles) Has(r Role) bool { return slices.Contains(rs, r) }

// Strings returns the role names.
func (rs Roles) Strings() []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}

func (rs Roles) anyLocaleScoped() bool { return slices.ContainsFunc(rs, Role.localeScoped) }

// Scope is a coarse capability an API token is created with.
type Scope string

// The token scopes. Every scope implies read: a token that may write or
// publish must be able to see what it writes or publishes.
const (
	// ScopeRead reads the tenant's catalog, translations, releases,
	// members and tokens.
	ScopeRead Scope = "read"
	// ScopeWrite pushes source messages, translations and linguistic
	// knowledge — terms, style guides, TM retirement (CLI push, CI
	// import, agents). Review is a human decision and no scope grants it.
	ScopeWrite Scope = "write"
	// ScopePublish creates releases.
	ScopePublish Scope = "publish"
	// ScopeAdmin manages the tenant, its members and its tokens — never
	// its owners.
	ScopeAdmin Scope = "admin"
)

var scopePermissions = map[Scope][]Permission{
	ScopeRead:    append(slices.Clone(readAll), PermTokensRead),
	ScopeWrite:   {PermCatalogWrite, PermTranslationsWrite, PermKnowledgeWrite},
	ScopePublish: {PermReleasesPublish},
	ScopeAdmin:   {PermTenantManage, PermMembersManage, PermTokensManage},
}

// Scopes is a non-empty, sorted, duplicate-free set of token scopes.
type Scopes []Scope

// ParseScopes validates, sorts and de-duplicates scope names.
func ParseScopes(names []string) (Scopes, error) {
	var out Scopes
	for _, n := range names {
		s := Scope(n)
		if _, ok := scopePermissions[s]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidScope, n)
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrInvalidScope)
	}
	slices.Sort(out)
	return out, nil
}

// Strings returns the scope names.
func (ss Scopes) Strings() []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = string(s)
	}
	return out
}

// Grant is what a principal may do in one tenant: a set of permissions,
// each for every locale or for a LocaleScope. The zero Grant allows
// nothing.
type Grant struct {
	perms map[Permission]LocaleScope
}

// GrantForMember derives a member's grant. The locale scope limits only
// locale-scoped permissions that come from translator or reviewer; the
// same permission from any other role is unrestricted.
func GrantForMember(roles Roles, locales LocaleScope) Grant {
	g := Grant{perms: map[Permission]LocaleScope{}}
	for _, r := range roles {
		for _, p := range rolePermissions[r] {
			scope := LocaleScope{}
			if p.LocaleScoped() && r.localeScoped() {
				scope = locales
			}
			g.add(p, scope)
		}
	}
	return g
}

// GrantForScopes derives an API token's grant. Tokens are never
// locale-scoped.
func GrantForScopes(scopes Scopes) Grant {
	g := Grant{perms: map[Permission]LocaleScope{}}
	for _, s := range scopes {
		for _, p := range slices.Concat(scopePermissions[ScopeRead], scopePermissions[s]) {
			g.add(p, LocaleScope{})
		}
	}
	return g
}

// GrantOf grants exactly perms, for every locale: a background
// process's grant (authz.Background), which no role or scope derives.
func GrantOf(perms ...Permission) Grant {
	g := Grant{perms: map[Permission]LocaleScope{}}
	for _, p := range perms {
		g.add(p, LocaleScope{})
	}
	return g
}

// add merges p into the grant: unrestricted wins, scopes union.
func (g Grant) add(p Permission, scope LocaleScope) {
	cur, ok := g.perms[p]
	switch {
	case !ok:
		g.perms[p] = scope
	case cur.All() || scope.All():
		g.perms[p] = LocaleScope{}
	default:
		merged, _ := ParseLocaleScope(append(cur.Strings(), scope.Strings()...))
		g.perms[p] = merged
	}
}

// Allows reports whether p is granted for every locale.
func (g Grant) Allows(p Permission) bool {
	scope, ok := g.perms[p]
	return ok && scope.All()
}

// AllowsFor reports whether p is granted for locale l.
func (g Grant) AllowsFor(p Permission, l Locale) bool {
	scope, ok := g.perms[p]
	return ok && scope.Covers(l)
}

// Locales returns the locales p is granted for; ok is false when p isn't
// granted at all. An unrestricted scope means every locale.
func (g Grant) Locales(p Permission) (scope LocaleScope, ok bool) {
	scope, ok = g.perms[p]
	return scope, ok
}

// Covers reports whether g allows, for every locale, everything other
// allows — the check that a token never exceeds its creator.
func (g Grant) Covers(other Grant) bool {
	for p := range other.perms {
		if !g.Allows(p) {
			return false
		}
	}
	return true
}

// Permissions lists the granted permissions, sorted.
func (g Grant) Permissions() []Permission {
	out := make([]Permission, 0, len(g.perms))
	for p := range g.perms {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}
