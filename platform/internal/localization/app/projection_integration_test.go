//go:build integration

package app_test

import (
	"testing"

	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	catalogdomain "github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/localization/app"
)

func keysOf(t *testing.T, h *harness, p catalogdomain.ProjectID, q catalogapp.MessageQuery) []string {
	t.Helper()
	ms, _, err := h.catalog.ListMessages(h.developer(), p, q, firstPage())
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, m := range ms {
		keys = append(keys, string(m.Key))
	}
	return keys
}

// A bulk upsert brings Localization's projection up to date in its own
// transaction: missing_in and outdated_in see the push at once, before
// any event is delivered, and the outbox catch-up later changes nothing
// — the outdated event is published once.
func TestBulkUpsertProjectsSynchronously(t *testing.T) {
	h := newHarness(t)
	p := catalogdomain.ProjectID(h.setup(t, false, []string{"de"}, map[string]string{"home.title": "Welcome"}))
	ctx := h.developer()
	if _, _, err := h.svc.PutTranslation(ctx, p.UUID(), "home.title", "de", app.TranslationInput{Text: "Willkommen"}, nil); err != nil {
		t.Fatal(err)
	}
	h.drain(t)
	outdatedEvents := func() int {
		return count(t, "SELECT count(*) FROM outbox_events WHERE event_type = 'localization.translation.outdated'")
	}

	res, err := h.catalog.UpsertMessages(ctx, p, []catalogapp.UpsertItem{
		{Key: "home.title", Text: "Welcome home"}, {Key: "home.greeting", Text: "Hello"},
	})
	if err != nil || res[0].Status != catalogapp.UpsertRevised || res[1].Status != catalogapp.UpsertCreated {
		t.Fatalf("upsert = %+v, %v", res, err)
	}
	// No event delivered yet.
	if got := keysOf(t, h, p, catalogapp.MessageQuery{MissingIn: "de"}); len(got) != 1 || got[0] != "home.greeting" {
		t.Errorf("missing_in right after the push = %v", got)
	}
	if got := keysOf(t, h, p, catalogapp.MessageQuery{OutdatedIn: "de"}); len(got) != 1 || got[0] != "home.title" {
		t.Errorf("outdated_in right after the push = %v", got)
	}
	if n := outdatedEvents(); n != 1 {
		t.Errorf("outdated events after the push = %d, want 1", n)
	}

	h.drain(t) // the catch-up finds the projection current
	if n := outdatedEvents(); n != 1 {
		t.Errorf("outdated events after catch-up = %d, want still 1", n)
	}
	if got := keysOf(t, h, p, catalogapp.MessageQuery{OutdatedIn: "de"}); len(got) != 1 {
		t.Errorf("outdated_in after catch-up = %v", got)
	}

	// Pushing the same catalog again changes nothing.
	if _, err := h.catalog.UpsertMessages(ctx, p, []catalogapp.UpsertItem{{Key: "home.title", Text: "Welcome home"}}); err != nil {
		t.Fatal(err)
	}
	if n := outdatedEvents(); n != 1 {
		t.Errorf("outdated events after a no-op push = %d", n)
	}
}
