//go:build integration

package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/intelligence/app"
	"github.com/felixgeelhaar/glossa/platform/internal/intelligence/domain"
	localizationapp "github.com/felixgeelhaar/glossa/platform/internal/localization/app"
)

// selectionProject has a.new (missing in de), b.old (outdated) and
// c.done (up to date).
func selectionProject(t *testing.T, w *wiring) uuid.UUID {
	t.Helper()
	p := w.project(t, []string{"de"}, map[string]string{"a.new": "New", "b.old": "Old", "c.done": "Done"})
	for key, text := range map[string]string{"b.old": "Alt", "c.done": "Fertig"} {
		if _, _, err := w.localization.PutTranslation(w.reviewer(), p, key, "de", localizationapp.TranslationInput{Text: text}, nil); err != nil {
			t.Fatal(err)
		}
	}
	w.push(t, p, map[string]string{"b.old": "Old, revised"})
	return p
}

// fillKeys requests a fill and returns the keys of its jobs, sorted.
// It cancels them again, so the next fill queues them anew (under its
// own ID).
func fillKeys(t *testing.T, w *wiring, p uuid.UUID, f app.FillFilter) ([]string, app.FillResult) {
	t.Helper()
	res, _, err := w.svc.RequestFill(w.developer(), p, app.FillRequest{Locales: []string{"de"}, Filter: f}, "")
	if err != nil {
		t.Fatal(err)
	}
	jobs, _, err := w.svc.ListJobs(w.developer(), app.JobFilter{FillID: &res.Fill.ID}, firstPageW())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.CancelFill(w.developer(), res.Fill.ID); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, j := range jobs {
		keys = append(keys, j.MessageKey)
	}
	slices.Sort(keys)
	return keys, res
}

// A fill selects messages by their translation's state — missing (the
// default), outdated, or both — with or without listed keys.
func TestWiringFillSelectsByState(t *testing.T) {
	w := newWiring(t, nil)
	w.configure(t, false, 0)
	p := selectionProject(t, w)

	for _, tc := range []struct {
		name   string
		filter app.FillFilter
		want   string
	}{
		{"default", app.FillFilter{}, "a.new"},
		{"missing", app.FillFilter{Select: app.SelectMissing}, "a.new"},
		{"outdated", app.FillFilter{Select: app.SelectOutdated}, "b.old"},
		{"both", app.FillFilter{Select: app.SelectMissingOrOutdated}, "a.new,b.old"},
		{"include_outdated", app.FillFilter{IncludeOutdated: true}, "a.new,b.old"},
		{"keys", app.FillFilter{Keys: []string{"a.new", "b.old", "c.done"}}, "a.new,b.old"},
		{"outdated keys", app.FillFilter{Keys: []string{"a.new", "b.old", "c.done"}, Select: app.SelectOutdated}, "b.old"},
		{"prefix", app.FillFilter{KeyPrefix: "a.", Select: app.SelectMissingOrOutdated}, "a.new"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys, res := fillKeys(t, w, p, tc.filter)
			if strings.Join(keys, ",") != tc.want {
				t.Errorf("keys = %v, want %s (fill %+v)", keys, tc.want, res.Fill)
			}
			if res.Fill.Filter.Select == "" {
				t.Errorf("the effective select isn't recorded: %+v", res.Fill.Filter)
			}
		})
	}
	_, res := fillKeys(t, w, p, app.FillFilter{Keys: []string{"a.new", "b.old", "c.done"}, Select: app.SelectOutdated})
	if res.Fill.Skipped[domain.SkipUpToDate] != 1 || res.Fill.Skipped[app.SkipNotSelected] != 1 {
		t.Errorf("skipped = %v, want c.done up to date and a.new not selected", res.Fill.Skipped)
	}

	for _, bad := range []app.FillFilter{
		{Select: "everything"},
		{Select: app.SelectMissing, IncludeOutdated: true},
	} {
		if _, _, err := w.svc.RequestFill(w.developer(), p, app.FillRequest{Locales: []string{"de"}, Filter: bad}, ""); !errors.Is(err, app.ErrInvalidQuery) {
			t.Errorf("filter %+v: err = %v, want ErrInvalidQuery", bad, err)
		}
	}
}
