package snapshot_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/config"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/remote"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func localConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{Version: 1, SourceLocale: "en", Catalogs: config.Catalogs{Path: "locales/{locale}.json"},
		Path: filepath.Join(dir, config.FileName)}
	return cfg
}

func TestFromLocalParsesCatalogsWithTheKernel(t *testing.T) {
	cfg := localConfig(t)
	writeFile(t, cfg.CatalogPath("en"), `{"checkout": {"pay": "Pay {amount, number}"}, "broken": "{oops"}`)
	writeFile(t, cfg.CatalogPath("de_de"), `{"checkout.pay": "Zahle {amount, number}"}`)
	writeFile(t, cfg.CatalogPath("ar"), `{"checkout.pay": "ادفع"}`)
	s, err := snapshot.FromLocal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Locales) != 3 || s.Locales[0].Code != "en" || !s.Locales[0].IsSource ||
		s.Locales[1].Code != "ar" || s.Locales[1].Direction != "rtl" || s.Locales[2].Code != "de-DE" {
		t.Fatalf("locales = %+v", s.Locales)
	}
	pay, ok := s.Message("checkout.pay")
	if !ok || pay.Model == nil || len(pay.Arguments) != 1 || pay.Arguments[0].Type != mf.ArgNumber {
		t.Fatalf("checkout.pay = %+v", pay)
	}
	broken, _ := s.Message("broken")
	if broken.Invalid == nil || broken.Invalid.Code != string(mf.CodeMF1SyntaxError) {
		t.Errorf("broken = %+v", broken.Invalid)
	}
	if tr := s.Translations["de-DE"]["checkout.pay"]; tr.Model == nil || tr.Text != "Zahle {amount, number}" {
		t.Errorf("de translation = %+v", tr)
	}
}

func TestFromLocalNeedsTheSourceCatalog(t *testing.T) {
	_, err := snapshot.FromLocal(localConfig(t))
	var missing *snapshot.SourceCatalogMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v", err)
	}
}

type fakeReader struct{}

func (fakeReader) Locales(context.Context, remote.Scope) ([]remote.ProjectLocale, error) {
	return []remote.ProjectLocale{{Code: "de"}, {Code: "en", IsSource: true}}, nil
}

func (fakeReader) Messages(_ context.Context, _ remote.Scope, f remote.MessageFilter) ([]remote.Message, error) {
	if f.State != "active" {
		return nil, errors.New("want active messages only")
	}
	m := remote.Message{Key: "cart.items", Namespace: "default", SourceRevision: 2}
	m.Source.Text = "{count, plural, one {# item} other {# items}}"
	m.Source.Syntax = "mf1"
	model, _ := mf.ParseMF1(m.Source.Text, "en")
	m.Source.Model = toMap(model)
	return []remote.Message{m}, nil
}

func (fakeReader) AllTranslations(_ context.Context, _ remote.Scope, keys []string) (map[string][]remote.Translation, error) {
	model, _ := mf.ParseMF1("{count, plural, one {# Artikel} other {# Artikel}}", "de")
	detail := "one"
	return map[string][]remote.Translation{keys[0]: {{Locale: "de", Text: "…", State: "approved", SourceRevision: 1,
		Outdated: true, Model: toMap(model), Warnings: []remote.QAFinding{{Code: "max-length-exceeded", Severity: "warning", Message: "long", Detail: &detail}}}}}, nil
}

func (fakeReader) FallbackGraph(context.Context, remote.Scope) (map[string][]string, error) {
	return map[string][]string{"*": {"en"}}, nil
}

func toMap(m mf.Message) map[string]any {
	var out map[string]any
	b, _ := m.MarshalJSON()
	_ = json.Unmarshal(b, &out)
	return out
}

func TestFromServerDecodesModelsAndTranslations(t *testing.T) {
	s, err := snapshot.FromServer(context.Background(), fakeReader{}, remote.Scope{}, "en", snapshot.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Locales[0].Code != "en" || len(s.TargetLocales()) != 1 || s.Fallback["*"][0] != "en" {
		t.Errorf("locales = %+v fallback = %v", s.Locales, s.Fallback)
	}
	m, _ := s.Message("cart.items")
	if m.Model == nil || len(m.Arguments) != 1 || m.Arguments[0].Selector == nil || m.Revision != 2 {
		t.Errorf("message = %+v", m)
	}
	tr := s.Translations["de"]["cart.items"]
	if tr.Model == nil || !tr.Outdated || tr.State != "approved" || len(tr.Warnings) != 1 || tr.Warnings[0].Detail != "one" {
		t.Errorf("translation = %+v", tr)
	}
}
