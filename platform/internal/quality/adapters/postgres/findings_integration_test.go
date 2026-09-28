//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db/dbtest"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
	qualitypg "github.com/felixgeelhaar/glossa/platform/internal/quality/adapters/postgres"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

var env *dbtest.Env

func TestMain(m *testing.M) {
	var err error
	env, err = dbtest.Start(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	env.Close()
	os.Exit(code)
}

// seedRun writes one completed check run and its findings. Quality's
// writer is a later slice, so the rows go in directly: what is under
// test here is the read, the SQL and the isolation.
func seedRun(
	t *testing.T, uow *db.UnitOfWork, tenant tenancy.ID, project uuid.UUID,
	ref string, started time.Time, findings []domain.Finding,
) uuid.UUID {
	t.Helper()
	run := uuid.Must(uuid.NewV7())
	err := uow.InTenantTx(tenancy.ContextWithTenant(t.Context(), tenant),
		func(ctx context.Context, tx *db.TenantTx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO quality_check_runs (id, tenant_id, project_id, ref, trigger, policy_version,
				    layers, errors, warnings, waived, conclusion, created_by, started_at, completed_at)
				VALUES ($1, $2, $3, $4, 'cli', 3, ARRAY['structure','parity'], 1, 1, 0, 'failure',
				        'token:seed', $5, $5)`,
				run, tenant.UUID(), project, ref, started)
			if err != nil {
				return err
			}
			for _, f := range findings {
				_, err := tx.Exec(ctx, `
					INSERT INTO quality_findings (id, tenant_id, run_id, project_id, fingerprint, layer, code,
					    severity, message_key, locale, namespace, file, line, explanation, subject)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
					uuid.Must(uuid.NewV7()), tenant.UUID(), run, project, f.Fingerprint, string(f.Layer), f.Code,
					string(f.Severity), f.Locus.Key, f.Locus.Locale, f.Locus.Namespace, f.Locus.File,
					f.Locus.Line, f.Message, f.Subject)
				if err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		t.Fatalf("seed a run: %v", err)
	}
	return run
}

func finding(layer domain.Layer, code, key, locale string, severity domain.Severity) domain.Finding {
	return domain.New(domain.Finding{
		Layer: layer, Code: code, Severity: severity, Subject: code,
		Locus:   domain.Locus{Key: key, Locale: locale, Namespace: "checkout", File: "src/Checkout.vue", Line: 12},
		Message: "something is wrong with " + key,
	})
}

func inTenant(t *testing.T, uow *db.UnitOfWork, tenant tenancy.ID, fn func(context.Context, app.Store) error) error {
	t.Helper()
	return qualitypg.NewTransactor(uow).InTenant(tenancy.ContextWithTenant(t.Context(), tenant), fn)
}

// Findings are tenant-owned under forced row-level security: one
// tenant's run is not visible to another, whatever id it asks for.
func TestFindingsAreTenantIsolated(t *testing.T) {
	ctx := t.Context()
	if err := env.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := env.SeedTenant(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := env.SeedTenant(ctx, "second")
	if err != nil {
		t.Fatal(err)
	}
	uow := db.NewUnitOfWork(env.App)
	now := time.Now().UTC().Truncate(time.Millisecond)
	projectA, projectB := uuid.New(), uuid.New()
	runA := seedRun(t, uow, first, projectA, "main", now, []domain.Finding{
		finding(domain.LayerParity, "argument_missing", "checkout.pay", "de", domain.Error),
	})
	seedRun(t, uow, second, projectB, "main", now, []domain.Finding{
		finding(domain.LayerStyle, "secret_code", "secret.key", "fr", domain.Warning),
	})

	// The second tenant's own project is simply not there for the first.
	err = inTenant(t, uow, first, func(ctx context.Context, st app.Store) error {
		if _, err := st.LatestRun(ctx, projectB, "", true); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("another tenant's project answered %v, want not found", err)
		}
		// Even naming the other tenant's run id directly returns nothing.
		got, err := st.Findings(ctx, runA, app.FindingFilter{}, "", 10)
		if err != nil {
			return err
		}
		if len(got) != 1 || got[0].Locus.Key != "checkout.pay" {
			t.Errorf("findings = %+v, want the first tenant's own", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = inTenant(t, uow, second, func(ctx context.Context, st app.Store) error {
		got, err := st.Findings(ctx, runA, app.FindingFilter{}, "", 10)
		if err != nil {
			return err
		}
		if len(got) != 0 {
			t.Errorf("the second tenant read the first's findings: %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The latest run wins, and a ref picks the latest of that ref.
func TestLatestRunPicksTheNewest(t *testing.T) {
	ctx := t.Context()
	if err := env.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	tenant, err := env.SeedTenant(ctx, "latest")
	if err != nil {
		t.Fatal(err)
	}
	uow := db.NewUnitOfWork(env.App)
	project := uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)
	seedRun(t, uow, tenant, project, "main", now.Add(-time.Hour), nil)
	newest := seedRun(t, uow, tenant, project, "pr-7", now, nil)
	main := seedRun(t, uow, tenant, project, "main", now.Add(-time.Minute), nil)

	err = inTenant(t, uow, tenant, func(ctx context.Context, st app.Store) error {
		latest, err := st.LatestRun(ctx, project, "", true)
		if err != nil {
			return err
		}
		if latest.ID != newest {
			t.Errorf("latest run = %v, want the newest %v", latest.ID, newest)
		}
		if latest.PolicyVersion != 3 || latest.Counts.Errors != 1 || latest.Conclusion != domain.ConclusionFailure {
			t.Errorf("run = %+v", latest)
		}
		if len(latest.Layers) != 2 || latest.Layers[0] != domain.LayerStructure {
			t.Errorf("layers = %v", latest.Layers)
		}
		if latest.CompletedAt == nil {
			t.Error("a completed run came back without its completion time")
		}
		ofRef, err := st.LatestRun(ctx, project, "main", true)
		if err != nil {
			return err
		}
		if ofRef.ID != main {
			t.Errorf("latest run of main = %v, want %v", ofRef.ID, main)
		}
		if _, err := st.LatestRun(ctx, project, "nope", true); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("an unknown ref answered %v, want not found", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Filters narrow, and the keyset cursor walks a run's findings without
// repeating or skipping one.
func TestFindingsFilterAndPaginate(t *testing.T) {
	ctx := t.Context()
	if err := env.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	tenant, err := env.SeedTenant(ctx, "filters")
	if err != nil {
		t.Fatal(err)
	}
	uow := db.NewUnitOfWork(env.App)
	project := uuid.New()
	run := seedRun(t, uow, tenant, project, "main", time.Now().UTC(), []domain.Finding{
		finding(domain.LayerParity, "argument_missing", "checkout.pay", "de", domain.Error),
		finding(domain.LayerParity, "argument_extra", "checkout.pay", "fr", domain.Warning),
		finding(domain.LayerStructure, "parse_error", "home.title", "de", domain.Error),
	})

	err = inTenant(t, uow, tenant, func(ctx context.Context, st app.Store) error {
		cases := []struct {
			name   string
			filter app.FindingFilter
			want   int
		}{
			{name: "everything", want: 3},
			{name: "by layer", filter: app.FindingFilter{Layer: domain.LayerParity}, want: 2},
			{name: "by locale", filter: app.FindingFilter{Locale: "de"}, want: 2},
			{name: "by severity", filter: app.FindingFilter{Severity: domain.Error}, want: 2},
			{name: "by key", filter: app.FindingFilter{MessageKey: "home.title"}, want: 1},
			{name: "waived only", filter: app.FindingFilter{WaivedOnly: true}, want: 0},
			{
				name:   "layer and locale together",
				filter: app.FindingFilter{Layer: domain.LayerParity, Locale: "fr"}, want: 1,
			},
		}
		for _, tc := range cases {
			got, err := st.Findings(ctx, run, tc.filter, "", 50)
			if err != nil {
				return err
			}
			if len(got) != tc.want {
				t.Errorf("%s: findings = %d, want %d", tc.name, len(got), tc.want)
			}
		}
		// The cursor: one row at a time, over the whole run.
		seen := map[uuid.UUID]bool{}
		after := ""
		for range 4 {
			got, err := st.Findings(ctx, run, app.FindingFilter{}, after, 1)
			if err != nil {
				return err
			}
			if len(got) == 0 {
				break
			}
			if seen[got[0].ID] {
				t.Fatalf("the cursor repeated %v", got[0].ID)
			}
			seen[got[0].ID] = true
			after = got[0].ID.String()
		}
		if len(seen) != 3 {
			t.Errorf("the cursor walked %d findings, want 3", len(seen))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A stored finding comes back whole enough to act on: its fingerprint
// as written, its locus, and its prose.
func TestFindingRoundTrip(t *testing.T) {
	ctx := t.Context()
	if err := env.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	tenant, err := env.SeedTenant(ctx, "roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	uow := db.NewUnitOfWork(env.App)
	project := uuid.New()
	want := finding(domain.LayerParity, "argument_missing", "checkout.pay", "de", domain.Error)
	run := seedRun(t, uow, tenant, project, "main", time.Now().UTC(), []domain.Finding{want})

	err = inTenant(t, uow, tenant, func(ctx context.Context, st app.Store) error {
		got, err := st.Findings(ctx, run, app.FindingFilter{}, "", 10)
		if err != nil {
			return err
		}
		if len(got) != 1 {
			t.Fatalf("findings = %d, want 1", len(got))
		}
		f := got[0]
		switch {
		case f.Fingerprint != want.Fingerprint:
			t.Errorf("fingerprint = %q, want the stored %q", f.Fingerprint, want.Fingerprint)
		case f.Schema != domain.Schema:
			t.Errorf("schema = %q", f.Schema)
		case f.Locus.File != "src/Checkout.vue" || f.Locus.Line != 12:
			t.Errorf("locus = %+v", f.Locus)
		case f.Message != want.Message || f.Subject != want.Subject:
			t.Errorf("finding = %+v", f)
		case f.ID == uuid.Nil:
			t.Error("the row has no id to page on")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
