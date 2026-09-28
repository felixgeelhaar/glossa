//go:build integration

package app_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogpg "github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/postgres"
	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	catalogdomain "github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz/authztest"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db/dbtest"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
	qualitycatalog "github.com/felixgeelhaar/glossa/platform/internal/quality/adapters/catalog"
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

// clock is a settable time source.
type clock struct{ t time.Time }

func newClock() *clock                   { return &clock{t: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)} }
func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// harness wires Catalog and Quality the way the composition root does.
type harness struct {
	catalog *catalogapp.Service
	svc     *app.Service
	tenant  tenancy.ID
	clock   *clock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	if err := env.Reset(context.Background()); err != nil {
		t.Fatalf("reset: %v", err)
	}
	return harnessFor(t, "acme")
}

// harnessFor adds a tenant to the current database state.
func harnessFor(t *testing.T, slug string) *harness {
	t.Helper()
	tenant, err := env.SeedTenant(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	uow := db.NewUnitOfWork(env.App)
	clk := newClock()
	cat := catalogapp.New(catalogpg.NewTransactor(uow), catalogapp.WithClock(clk.now))
	svc := app.NewService(qualitypg.NewTransactor(uow), qualitycatalog.New(cat), app.WithClock(clk.now))
	return &harness{catalog: cat, svc: svc, tenant: tenant, clock: clk}
}

func (h *harness) developer() context.Context {
	return authztest.Member(context.Background(), h.tenant, []string{"developer"})
}

func (h *harness) translator() context.Context {
	return authztest.Member(context.Background(), h.tenant, []string{"translator"}, "de")
}

// project creates a project and returns its ID.
func (h *harness) project(t *testing.T, slug string) uuid.UUID {
	t.Helper()
	settings := catalogdomain.Settings{DefaultSyntax: mfcontent.MF1}
	p, _, err := h.catalog.CreateProject(h.developer(),
		catalogapp.NewProject{Slug: slug, Name: slug, SourceLocale: "en", Settings: &settings}, "")
	if err != nil {
		t.Fatal(err)
	}
	return p.ID.UUID()
}

// sha is a full commit ID from a short one.
func sha(short string) string { return short + strings.Repeat("0", 40-len(short)) }

// finding builds a sealed finding, so every test's fingerprints are the
// ones the layers would compute.
func finding(layer domain.Layer, code string, l domain.Locus, severity domain.Severity, opts ...func(*domain.Finding)) domain.Finding {
	f := domain.Finding{Layer: layer, Code: code, Locus: l, Severity: severity, Message: code + " in " + l.Key}
	for _, o := range opts {
		o(&f)
	}
	return domain.New(f)
}

func at(revision int) func(*domain.Finding) {
	return func(f *domain.Finding) { f.SourceRevision = &revision }
}

func subject(s string) func(*domain.Finding) {
	return func(f *domain.Finding) { f.Subject = s }
}
