//go:build integration

package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

func TestInTenantTxScopesTheTransaction(t *testing.T) {
	reset(t)
	tenant := seedTenant(t, "acme")
	ctx := tenancy.ContextWithTenant(context.Background(), tenant)

	err := db.NewUnitOfWork(env.App).InTenantTx(ctx, func(ctx context.Context, tx *db.TenantTx) error {
		if tx.Tenant() != tenant {
			t.Errorf("Tenant() = %v, want %v", tx.Tenant(), tenant)
		}
		var got string
		if err := tx.QueryRow(ctx, "SELECT app_current_tenant()::text").Scan(&got); err != nil {
			return err
		}
		if got != tenant.String() {
			t.Errorf("app_current_tenant() = %s, want %s", got, tenant)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The setting is transaction-local: the pooled connection is clean.
	var leaked *string
	if err := env.App.QueryRow(context.Background(), "SELECT app_current_tenant()::text").Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != nil {
		t.Errorf("tenant leaked onto the pooled connection: %s", *leaked)
	}
}

func TestInTenantTxRollsBackOnErrorAndPanic(t *testing.T) {
	reset(t)
	tenant := seedTenant(t, "acme")
	ctx := tenancy.ContextWithTenant(context.Background(), tenant)
	uow := db.NewUnitOfWork(env.App)
	boom := errors.New("boom")

	err := uow.InTenantTx(ctx, func(ctx context.Context, tx *db.TenantTx) error {
		if err := insertEvent(ctx, tx, tenant); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed")
			}
		}()
		_ = uow.InTenantTx(ctx, func(ctx context.Context, tx *db.TenantTx) error {
			if err := insertEvent(ctx, tx, tenant); err != nil {
				return err
			}
			panic("handler bug")
		})
	}()

	err = uow.InTenantTx(ctx, func(ctx context.Context, tx *db.TenantTx) error {
		n, err := countRows(ctx, tx, "outbox_events")
		if err == nil && n != 0 {
			t.Errorf("%d events survived rollback", n)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNestedUnitOfWorkIsRefused(t *testing.T) {
	reset(t)
	tenant := seedTenant(t, "acme")
	ctx := tenancy.ContextWithTenant(context.Background(), tenant)
	uow := db.NewUnitOfWork(env.App)

	err := uow.InTenantTx(ctx, func(ctx context.Context, _ *db.TenantTx) error {
		return uow.InTenantTx(ctx, func(context.Context, *db.TenantTx) error { return nil })
	})
	if !errors.Is(err, db.ErrNestedTx) {
		t.Fatalf("err = %v, want ErrNestedTx", err)
	}
}

func TestSystemScopeReachesOnlyGrantedTables(t *testing.T) {
	reset(t)
	a, b := seedTenant(t, "acme"), seedTenant(t, "bolt")
	seedEvent(t, a)
	seedEvent(t, b)
	uow := db.NewUnitOfWork(env.App)
	scope := db.NewSystemScope("test.relay")

	err := uow.InSystemTx(context.Background(), scope, func(ctx context.Context, tx *db.SystemTx) error {
		n, err := countRows(ctx, tx, "outbox_events")
		if err != nil {
			return err
		}
		if n != 2 {
			t.Errorf("system scope sees %d events, want 2 (all tenants)", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// tenants isn't opened to glossa_system: permission denied.
	err = uow.InSystemTx(context.Background(), scope, func(ctx context.Context, tx *db.SystemTx) error {
		_, err := countRows(ctx, tx, "tenants")
		return err
	})
	if err == nil {
		t.Error("system scope could read tenants")
	}

	// It can't forge events either (no INSERT grant or policy).
	err = uow.InSystemTx(context.Background(), scope, func(ctx context.Context, tx *db.SystemTx) error {
		return insertEvent(ctx, tx, a)
	})
	if err == nil {
		t.Error("system scope could insert an outbox event")
	}
}

func TestRequestRoleCannotReachSystemPoliciesWithoutSwitching(t *testing.T) {
	reset(t)
	a := seedTenant(t, "acme")
	seedEvent(t, a)
	// glossa_app is a member of glossa_system WITH INHERIT FALSE, so the
	// relay's permissive policies don't apply to it on a plain connection.
	n, err := countRows(context.Background(), env.App, "outbox_events")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("glossa_app sees %d events outside any scope", n)
	}
}
