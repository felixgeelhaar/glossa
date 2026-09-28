package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz/authztest"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// ── a store that remembers what it was asked ────────────────────────

type fakeStore struct {
	run      app.RunSummary
	findings []app.StoredFinding
	runErr   error
	// lastLimit and lastFilter are what ListFindings passed down.
	lastLimit  int
	lastFilter app.FindingFilter
	lastAfter  string
	lastRef    string
}

func (f *fakeStore) LatestRun(_ context.Context, _ uuid.UUID, ref string, completedOnly bool) (app.RunSummary, error) {
	f.lastRef = ref
	if !completedOnly {
		return app.RunSummary{}, errors.New("a read surface must not show a run still in flight")
	}
	return f.run, f.runErr
}

func (f *fakeStore) Findings(
	_ context.Context, _ uuid.UUID, filter app.FindingFilter, after string, limit int,
) ([]app.StoredFinding, error) {
	f.lastFilter, f.lastAfter, f.lastLimit = filter, after, limit
	if len(f.findings) > limit {
		return f.findings[:limit], nil
	}
	return f.findings, nil
}

type fakeTx struct{ store *fakeStore }

func (t fakeTx) InTenant(ctx context.Context, fn func(context.Context, app.Store) error) error {
	return fn(ctx, t.store)
}

func storeWith(n int) *fakeStore {
	s := &fakeStore{run: app.RunSummary{
		ID: uuid.New(), Ref: "main", Trigger: domain.TriggerCLI, Conclusion: domain.ConclusionFailure,
		Layers: []domain.Layer{domain.LayerParity}, Counts: domain.Counts{Errors: 1},
		StartedAt: time.Now().UTC(),
	}}
	for i := range n {
		s.findings = append(s.findings, app.StoredFinding{
			ID: uuid.New(),
			Finding: domain.New(domain.Finding{
				Layer: domain.LayerParity, Code: "argument_missing", Severity: domain.Error,
				Locus: domain.Locus{Key: "checkout.pay", Locale: "de"}, Subject: string(rune('a' + i)),
			}),
		})
	}
	return s
}

// ctx returns a context carrying a principal that may read the catalog.
func ctx(t *testing.T) context.Context {
	t.Helper()
	return authztest.Token(t.Context(), tenancy.NewID(), "read")
}

func TestListFindingsReadsTheLatestCompletedRun(t *testing.T) {
	store := storeWith(3)
	svc := app.NewService(fakeTx{store: store})

	page, err := svc.ListFindings(ctx(t), uuid.New(), app.FindingQuery{Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Run.Ref != "main" || len(page.Findings) != 3 || page.Next != "" {
		t.Fatalf("page = %+v", page)
	}
	if store.lastRef != "main" {
		t.Fatalf("ref = %q", store.lastRef)
	}
	if store.lastLimit != app.DefaultFindingLimit+1 {
		t.Fatalf("limit = %d, want the default plus the lookahead row", store.lastLimit)
	}
}

func TestListFindingsPaginates(t *testing.T) {
	store := storeWith(5)
	svc := app.NewService(fakeTx{store: store})

	page, err := svc.ListFindings(ctx(t), uuid.New(), app.FindingQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Findings) != 2 {
		t.Fatalf("findings = %d, want 2", len(page.Findings))
	}
	if page.Next != page.Findings[1].ID.String() {
		t.Fatalf("cursor = %q, want the last row's id", page.Next)
	}
	if _, err := svc.ListFindings(ctx(t), uuid.New(), app.FindingQuery{Limit: 2, After: page.Next}); err != nil {
		t.Fatal(err)
	}
	if store.lastAfter != page.Next {
		t.Fatalf("after = %q, want the cursor", store.lastAfter)
	}
}

func TestListFindingsRefusesAnUnknownFilter(t *testing.T) {
	svc := app.NewService(fakeTx{store: storeWith(1)})
	tests := []struct {
		name  string
		query app.FindingQuery
	}{
		{name: "a layer nobody stores", query: app.FindingQuery{FindingFilter: app.FindingFilter{Layer: "spelling"}}},
		{name: "a severity nobody stores", query: app.FindingQuery{FindingFilter: app.FindingFilter{Severity: "info"}}},
		{name: "a limit past the cap", query: app.FindingQuery{Limit: app.MaxFindingLimit + 1}},
		{name: "a negative limit", query: app.FindingQuery{Limit: -1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// A filter outside the vocabulary would return nothing, which
			// reads like "clean" and is not.
			if _, err := svc.ListFindings(ctx(t), uuid.New(), tc.query); !errors.Is(err, app.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

func TestListFindingsPassesTheFilterDown(t *testing.T) {
	store := storeWith(1)
	svc := app.NewService(fakeTx{store: store})
	want := app.FindingFilter{
		Layer: domain.LayerParity, Locale: "de", Severity: domain.Error,
		MessageKey: "checkout.pay", WaivedOnly: true,
	}
	if _, err := svc.ListFindings(ctx(t), uuid.New(), app.FindingQuery{FindingFilter: want}); err != nil {
		t.Fatal(err)
	}
	if store.lastFilter != want {
		t.Fatalf("filter = %+v, want %+v", store.lastFilter, want)
	}
}

func TestListFindingsNeedsCatalogRead(t *testing.T) {
	svc := app.NewService(fakeTx{store: storeWith(1)})
	if _, err := svc.ListFindings(context.Background(), uuid.New(), app.FindingQuery{}); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Fatalf("err = %v, want unauthenticated", err)
	}
}

func TestListFindingsPassesNotFoundOn(t *testing.T) {
	store := storeWith(0)
	store.runErr = app.ErrNotFound
	svc := app.NewService(fakeTx{store: store})
	if _, err := svc.ListFindings(ctx(t), uuid.New(), app.FindingQuery{}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
