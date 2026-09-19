// Package authz is how every bounded context asks "may the caller do
// this here?". Identity's HTTP edge puts a Principal on the request
// context after authenticating the caller and checking the tenant in the
// path against their membership or token; application services then
// call Require before acting:
//
//	if err := authz.Require(ctx, authz.CatalogWrite); err != nil {
//		return err // errors.Is(err, authz.ErrForbidden)
//	}
//	if err := authz.RequireFor(ctx, authz.TranslationsWrite, locale); err != nil { … }
//
// Checks are explicit and local: a permission, optionally a locale. There
// is no policy language and no remote call. The role matrix and token
// scopes that produce a Grant live in the Identity domain.
package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// Permission is re-exported so other contexts depend on this package
// only, not on Identity's domain.
type Permission = domain.Permission

// Locale is the locale RequireFor checks, re-exported for the same
// reason.
type Locale = domain.Locale

// ParseLocale canonicalizes a BCP 47 tag for RequireFor.
func ParseLocale(s string) (Locale, error) { return domain.ParseLocale(s) }

// The permissions other contexts check.
const (
	TenantRead         = domain.PermTenantRead
	TenantManage       = domain.PermTenantManage
	MembersRead        = domain.PermMembersRead
	MembersManage      = domain.PermMembersManage
	OwnersManage       = domain.PermOwnersManage
	TokensRead         = domain.PermTokensRead
	TokensManage       = domain.PermTokensManage
	CatalogRead        = domain.PermCatalogRead
	CatalogWrite       = domain.PermCatalogWrite
	KnowledgeRead      = domain.PermKnowledgeRead
	KnowledgeWrite     = domain.PermKnowledgeWrite
	TranslationsRead   = domain.PermTranslationsRead
	TranslationsWrite  = domain.PermTranslationsWrite
	TranslationsReview = domain.PermTranslationsReview
	ReleasesRead       = domain.PermReleasesRead
	ReleasesPublish    = domain.PermReleasesPublish
	// IntelligenceRead, IntelligenceManage and IntelligenceTranslate are
	// the AI permissions (RFC 0003 §6); IntelligenceTranslate is
	// locale-scoped.
	IntelligenceRead      = domain.PermIntelligenceRead
	IntelligenceManage    = domain.PermIntelligenceManage
	IntelligenceTranslate = domain.PermIntelligenceTranslate
	// IntegrationRead, IntegrationImport and IntegrationManage are the
	// import/export permissions (RFC 0003 §5–§6); IntegrationImport is
	// locale-scoped.
	IntegrationRead   = domain.PermIntegrationRead
	IntegrationImport = domain.PermIntegrationImport
	IntegrationManage = domain.PermIntegrationManage
)

var (
	// ErrUnauthenticated means the context carries no principal.
	ErrUnauthenticated = errors.New("authz: unauthenticated")
	// ErrForbidden means the principal lacks the permission here.
	ErrForbidden = errors.New("authz: forbidden")
)

// DeniedError says which permission was missing. It matches ErrForbidden.
type DeniedError struct {
	Permission Permission
	Locale     string // empty unless the check was for a locale
}

func (e *DeniedError) Error() string {
	if e.Locale != "" {
		return fmt.Sprintf("authz: %s is not granted for %s", e.Permission, e.Locale)
	}
	return fmt.Sprintf("authz: %s is not granted", e.Permission)
}

// Is makes errors.Is(err, ErrForbidden) true.
func (e *DeniedError) Is(target error) bool { return target == ErrForbidden }

// Principal is the authenticated caller.
type Principal struct {
	// Actor is the person or API token acting.
	Actor domain.Actor
	// Person is set when a person is acting (session), zero for tokens.
	Person domain.PersonID
	// Tenant is the tenant the grant applies to; zero outside tenant
	// routes, where the grant is empty.
	Tenant tenancy.ID
	// Member is the person's membership in Tenant, when there is one.
	Member domain.MemberID
	// TokenTenant is the tenant an API token belongs to, also on
	// tenantless routes; zero for people.
	TokenTenant tenancy.ID
	// Grant is what the principal may do in Tenant.
	Grant domain.Grant
}

type ctxKey struct{}

// WithPrincipal returns a context carrying p. Only Identity's HTTP edge
// (and tests) should call it.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// From returns the principal on ctx.
func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// Authenticated returns the principal for operations that read no
// tenant data and so need no permission, only a caller (a person or an
// API token) — stateless tools such as the message preview. Anything
// tenant-scoped uses Require or RequireFor instead.
func Authenticated(ctx context.Context) (Principal, error) {
	p, ok := From(ctx)
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	return p, nil
}

// identityAdministration are permissions background work never holds.
var identityAdministration = []Permission{
	domain.PermTenantManage, domain.PermMembersManage, domain.PermOwnersManage, domain.PermTokensManage,
}

// Background returns ctx acting as a bounded context's background
// process — an outbox subscriber or a job, named stably
// ("knowledge.derive_tm") — in the tenant already on ctx, holding
// exactly perms. It is how such work reads (or writes) another context
// through that context's authorized application ports instead of
// around them: the ports keep checking permissions, and the grant
// states what the process may do. It refuses a context without a
// tenant, one that already carries a principal (a request's caller is
// never swapped for a broader one) and identity administration.
func Background(ctx context.Context, name string, perms ...Permission) (context.Context, error) {
	if name == "" {
		return nil, errors.New("authz: a background principal needs a name")
	}
	tenant, ok := tenancy.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("authz: background %s: no tenant on the context", name)
	}
	if _, ok := From(ctx); ok {
		return nil, fmt.Errorf("authz: background %s: the context already carries a principal", name)
	}
	for _, p := range perms {
		for _, admin := range identityAdministration {
			if p == admin {
				return nil, fmt.Errorf("authz: background %s: %s is never granted to background work", name, p)
			}
		}
	}
	return WithPrincipal(ctx, Principal{
		Actor: domain.SystemActor(name), Tenant: tenant, Grant: domain.GrantOf(perms...),
	}), nil
}

// Require returns nil if the principal holds perm for every locale in
// the context's tenant.
func Require(ctx context.Context, perm Permission) error {
	p, err := inTenant(ctx)
	if err != nil {
		return err
	}
	if !p.Grant.Allows(perm) {
		return &DeniedError{Permission: perm}
	}
	return nil
}

// RequireFor returns nil if the principal holds perm for locale in the
// context's tenant. Use it for locale-scoped work (translate, review).
func RequireFor(ctx context.Context, perm Permission, locale domain.Locale) error {
	p, err := inTenant(ctx)
	if err != nil {
		return err
	}
	if !p.Grant.AllowsFor(perm, locale) {
		return &DeniedError{Permission: perm, Locale: locale.String()}
	}
	return nil
}

// inTenant returns the principal, refusing one whose grant belongs to a
// different tenant than the one the context (and so the database
// transaction) is scoped to.
func inTenant(ctx context.Context) (Principal, error) {
	p, ok := From(ctx)
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	tenant, ok := tenancy.FromContext(ctx)
	if !ok || tenant != p.Tenant {
		return Principal{}, fmt.Errorf("%w: principal is not scoped to this tenant", ErrForbidden)
	}
	return p, nil
}
