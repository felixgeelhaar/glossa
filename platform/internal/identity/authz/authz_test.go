package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

func principal(t *testing.T, tenant tenancy.ID, roles []string, locales []string) authz.Principal {
	t.Helper()
	r, err := domain.ParseRoles(roles)
	if err != nil {
		t.Fatal(err)
	}
	l, err := domain.ParseLocaleScope(locales)
	if err != nil {
		t.Fatal(err)
	}
	person := domain.NewPersonID()
	return authz.Principal{
		Actor: domain.PersonActor(person), Person: person, Tenant: tenant,
		Grant: domain.GrantForMember(r, l),
	}
}

func TestRequire(t *testing.T) {
	tenant := tenancy.NewID()
	ctx := tenancy.ContextWithTenant(context.Background(), tenant)
	ctx = authz.WithPrincipal(ctx, principal(t, tenant, []string{"translator"}, []string{"de"}))

	if err := authz.Require(ctx, authz.CatalogRead); err != nil {
		t.Errorf("translator reads the catalog: %v", err)
	}
	err := authz.Require(ctx, authz.CatalogWrite)
	var denied *authz.DeniedError
	if !errors.Is(err, authz.ErrForbidden) || !errors.As(err, &denied) || denied.Permission != authz.CatalogWrite {
		t.Errorf("translator writing the catalog err = %v", err)
	}

	de, _ := domain.ParseLocale("de-CH")
	ja, _ := domain.ParseLocale("ja")
	if err := authz.RequireFor(ctx, authz.TranslationsWrite, de); err != nil {
		t.Errorf("translator scoped to de writes de-CH: %v", err)
	}
	if err := authz.RequireFor(ctx, authz.TranslationsWrite, ja); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("translator scoped to de writing ja err = %v", err)
	}
}

func TestRequireWithoutPrincipal(t *testing.T) {
	ctx := tenancy.ContextWithTenant(context.Background(), tenancy.NewID())
	if err := authz.Require(ctx, authz.TenantRead); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrUnauthenticated", err)
	}
}

// A grant belongs to one tenant: it must not authorize work in another,
// even if a bug put the two side by side on one context.
func TestRequireRefusesAGrantFromAnotherTenant(t *testing.T) {
	mine, other := tenancy.NewID(), tenancy.NewID()
	ctx := tenancy.ContextWithTenant(context.Background(), other)
	ctx = authz.WithPrincipal(ctx, principal(t, mine, []string{"owner"}, nil))
	if err := authz.Require(ctx, authz.TenantRead); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
	if err := authz.Require(authz.WithPrincipal(context.Background(), principal(t, mine, []string{"owner"}, nil)), authz.TenantRead); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("no tenant on the context: err = %v, want ErrForbidden", err)
	}
}

// Tenantless operations (stateless tools) need only an authenticated
// caller, person or token.
func TestAuthenticated(t *testing.T) {
	if _, err := authz.Authenticated(context.Background()); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("no principal: err = %v", err)
	}
	p := principal(t, tenancy.ID{}, []string{"owner"}, nil)
	got, err := authz.Authenticated(authz.WithPrincipal(context.Background(), p))
	if err != nil || got.Actor != p.Actor {
		t.Errorf("person: %+v, %v", got, err)
	}
}
