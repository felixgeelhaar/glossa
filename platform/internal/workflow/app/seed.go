package app

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
	"github.com/felixgeelhaar/glossa/platform/internal/workflow/defaults"
	"github.com/felixgeelhaar/glossa/platform/internal/workflow/domain"
)

// EnsureDefault seeds the default review definition
// (defaults/review.json) for the tenant on ctx, once (RFC 0006 §2.3).
// It is a document like any other: compiled and stored as version 1 of
// a tenant-wide definition, editable and versioned from then on. It is
// not bound to anything — a project without a binding behaves as after
// M4 — and a tenant that deletes it is never given it back. A tenant
// that already has a tenant-wide definition called "review" keeps its
// own.
//
// "First use" is the tenant's creation (identity.tenant.created) or,
// for tenants that existed before the runner, the first workflow event
// it handles there. It is safe to call from anywhere and at any time.
func (r *Runner) EnsureDefault(ctx context.Context) error {
	tenant, ok := tenancy.FromContext(ctx)
	if !ok {
		return errors.New("workflow: seeding needs a tenant")
	}
	if _, done := r.seeded.Load(tenant); done {
		return nil
	}
	d, err := domain.Compile(defaults.Review())
	if err != nil {
		return err // the embedded document is the platform's own; a test keeps it compiling
	}
	by := authz.SystemEventActor(PrincipalSeed).String()
	now := r.now()
	err = r.d.Tx.InTenant(ctx, func(ctx context.Context, st InstanceStore) error {
		seeded, err := st.Seeded(ctx)
		if err != nil || seeded {
			return err
		}
		taken, err := st.HasTenantDefinition(ctx, d.Name)
		if err != nil {
			return err
		}
		var (
			rec *domain.DefinitionRecord
			ver *domain.Version
		)
		if !taken {
			rec = &domain.DefinitionRecord{ID: uuid.New(), Name: d.Name, Subject: d.Subject, Latest: 1, CreatedBy: by, CreatedAt: now}
			v := domain.FirstVersion(*rec, d, by, now)
			ver = &v
		}
		_, err = st.SeedDefault(ctx, rec, ver, now)
		return err
	})
	if errors.Is(err, ErrConflict) {
		err = nil // a concurrent seeding won
	}
	if err == nil {
		r.seeded.Store(tenant, struct{}{})
	}
	return err
}
