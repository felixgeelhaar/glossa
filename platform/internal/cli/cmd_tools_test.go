package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/release"
)

func TestGenerateWritesTypedAccessorsAndChecksStaleness(t *testing.T) {
	w := newWorkspace(t).withProject(nil, map[string]string{"en": sourceEN})
	var out generateJSON
	w.json(&out, "generate").want(t, ExitOK)
	if out.Messages != 3 || len(out.Files) != 3 || !out.Files[0].Changed {
		t.Fatalf("generate = %+v", out)
	}
	ts := w.read("src/glossa/messages.ts")
	for _, want := range []string{`"checkout.pay": { amount: number };`, "pay: (values: Messages[\"checkout.pay\"]): string"} {
		if !strings.Contains(ts, want) {
			t.Errorf("messages.ts lacks %q", want)
		}
	}
	if vue := w.read("src/glossa/glossa-vue.ts"); !strings.Contains(vue, `from "./messages.js"`) {
		t.Errorf("glossa-vue.ts = %s", vue)
	}
	if goSrc := w.read("internal/msg/messages.go"); !strings.Contains(goSrc, "package msg") ||
		!strings.Contains(goSrc, "func (m Messages) CheckoutPay(amount float64, opts ...glossa.Option) string") {
		t.Errorf("messages.go = %s", goSrc)
	}
	w.json(&out, "generate", "--check").want(t, ExitOK)
	w.write("locales/en.json", `{"checkout.pay": "Pay {total, number}"}`)
	r := w.run("generate", "--check")
	r.want(t, ExitCheckFailed)
	if !strings.Contains(r.stdout, "is stale") {
		t.Errorf("--check output = %s", r.stdout)
	}
}

func TestGenerateNeedsOutputs(t *testing.T) {
	w := newWorkspace(t)
	w.write("glossa.yaml", "version: 1\nserver: x\nproject: p\nsource_locale: en\ncatalogs:\n  path: locales/{locale}.json\n")
	w.write("locales/en.json", `{}`)
	var doc errorDoc
	w.json(&doc, "generate").want(t, ExitUsage)
	if doc.Error.Code != "nothing_to_generate" {
		t.Errorf("error = %+v", doc.Error)
	}
}

func TestExtractReportsUnknownAndUnused(t *testing.T) {
	w := newWorkspace(t).withProject(nil, map[string]string{"en": sourceEN})
	w.write("src/App.vue", `<template><p>{{ $t("cart.items", { count }) }} {{ t("cart.totla") }}</p></template>
<script setup lang="ts">
const m = useTypedMessages();
m.checkout.pay({ amount });
</script>`)
	w.write("internal/mail/mail.go", `package mail
func f() { _ = client.T(ctx, "cart.totla", nil) }`)
	var out extractJSON
	w.json(&out, "extract").want(t, ExitOK)
	if out.Files != 2 || len(out.Usages) != 4 {
		t.Fatalf("extract = %+v", out)
	}
	if len(out.Unknown) != 1 || out.Unknown[0].Key != "cart.totla" || len(out.Unknown[0].Locations) != 2 {
		t.Errorf("unknown = %+v", out.Unknown)
	}
	if strings.Join(out.Unused, ",") != "cart.checkout" {
		t.Errorf("unused = %v", out.Unused)
	}
	w.run("extract", "--strict").want(t, ExitCheckFailed)
}

func TestReleaseCommandsParseArgumentsAndSayTheyArentAvailableYet(t *testing.T) {
	srv := newFakeServer(t)
	w := newWorkspace(t).withProject(srv, map[string]string{"en": sourceEN})
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"release"}, "invalid_usage"},
		{[]string{"release", "ship"}, "invalid_usage"},
		{[]string{"release", "promote", "rel_1"}, "invalid_usage"},
		{[]string{"release", "rollback"}, "invalid_usage"},
		{[]string{"release", "keys", "extra"}, "invalid_usage"},
		{[]string{"release", "publish"}, "release_api_unavailable"},
		{[]string{"release", "promote", "rel_1", "--to", "staging"}, "release_api_unavailable"},
		{[]string{"release", "rollback", "--environment", "production"}, "release_api_unavailable"},
		{[]string{"release", "environments"}, "release_api_unavailable"},
		{[]string{"pull", "--release", "rel_1"}, "release_api_unavailable"},
	} {
		var doc errorDoc
		w.json(&doc, tc.args...).want(t, ExitUsage)
		if doc.Error.Code != tc.code {
			t.Errorf("%v: code = %q, want %q (%+v)", tc.args, doc.Error.Code, tc.code, doc.Error)
		}
	}
	inv := &invocation{env: Env{}, name: "release"}
	r, err := parseReleaseArgs(inv, []string{"publish", "--note", "first"})
	if err != nil || r.environment != "production" || r.note != "first" {
		t.Errorf("publish args = %+v, %v", r, err)
	}
	r, err = parseReleaseArgs(&invocation{name: "release"}, []string{"promote", "rel_9", "--to", "staging"})
	if err != nil || r.releaseID != "rel_9" || r.environment != "staging" {
		t.Errorf("promote args = %+v, %v", r, err)
	}
}

// fakeReleases serves one bundle, as the Release API will.
type fakeReleases struct {
	release.Unavailable
	manifest  []byte
	artifacts map[string][]byte
}

func (f fakeReleases) BundleSource(release.Scope, string) release.BundleSource { return f }
func (f fakeReleases) Manifest(context.Context) ([]byte, error)                { return f.manifest, nil }
func (f fakeReleases) Artifact(_ context.Context, sha string) ([]byte, error) {
	return f.artifacts[sha], nil
}

func TestPullReleaseWritesTheBundleLayout(t *testing.T) {
	srv := newFakeServer(t)
	w := newWorkspace(t).withProject(srv, map[string]string{"en": sourceEN})
	art := []byte(`{"schema":"glossa.artifact/v1","locale":"en","namespace":"default","messages":{}}`)
	sum := sha256.Sum256(art)
	sha := hex.EncodeToString(sum[:])
	w.releases = fakeReleases{
		manifest:  []byte(fmt.Sprintf(`{"schema":"glossa.manifest/v1","release":{"id":"rel_1","version":3},"artifacts":{"en":{"default":{"sha256":%q,"size":%d}}}}`, sha, len(art))),
		artifacts: map[string][]byte{sha: art},
	}
	var out pullJSON
	w.json(&out, "pull", "--release", "rel_1", "--out", "public/glossa").want(t, ExitOK)
	if out.Release == nil || out.Release.Version != 3 || out.Release.Artifacts != 1 {
		t.Fatalf("pull --release = %+v", out)
	}
	if _, err := os.Stat(filepath.Join(w.dir, "public", "glossa", "a", sha+".json")); err != nil {
		t.Error(err)
	}
}

// fakeV03 is a Glossa v0.3 API with a de source and an en locale.
func fakeV03(t *testing.T) *httptest.Server {
	t.Helper()
	bundles := map[string]string{
		"de": `{"project":"site","locale":"de","messages":{"cart.items":"{count, plural, one {# Artikel} other {# Artikel}}","home.title":"Willkommen","legal.note":""},"statuses":{"cart.items":"approved","home.title":"approved"}}`,
		"en": `{"project":"site","locale":"en","messages":{"cart.items":"{count, plural, one {# item} other {# items}}","home.title":"Welcome","legal.note":""},"statuses":{"cart.items":"approved","home.title":"ai_translated"}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer glossa_v03key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/projects/site/locales":
			_, _ = io.WriteString(w, `[{"code":"de"},{"code":"en"}]`)
		case strings.HasPrefix(r.URL.Path, "/api/v1/projects/site/locales/"):
			locale := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/site/locales/"), "/messages")
			_, _ = io.WriteString(w, bundles[locale])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestImportFromV03IsIdempotentAndMapsStatuses(t *testing.T) {
	srv := newFakeServer(t)
	srv.sourceLocale, srv.locales = "de", []string{"de"}
	old := fakeV03(t)
	w := newWorkspace(t).withProject(srv, nil)
	w.write("glossa.yaml", strings.Replace(w.read("glossa.yaml"), "source_locale: en", "source_locale: de", 1))
	w.env["GLOSSA_V0_KEY"] = "glossa_v03key"
	args := []string{"import", "--from", "v0", "--v0-url", old.URL, "--v0-project", "site"}

	var dry importJSON
	w.json(&dry, append(args, "--dry-run")...).want(t, ExitOK)
	if dry.Summary["message"]["planned"] != 2 || dry.Summary["translation"]["planned"] != 2 || srv.countRequests("POST") != 0 {
		t.Fatalf("dry run = %+v", dry.Summary)
	}

	var out importJSON
	w.json(&out, args...).want(t, ExitOK)
	if strings.Join(out.LocalesAdded, ",") != "en" || out.Summary["message"]["created"] != 2 || out.Summary["translation"]["created"] != 2 {
		t.Fatalf("import = %+v", out)
	}
	byKey := map[string]importItem{}
	for _, it := range out.Items {
		if it.Kind == "translation" {
			byKey[it.Key] = it
		}
	}
	// approved in v0.3, but a token can't approve where review is required.
	if it := byKey["cart.items"]; it.V0Status != "approved" || it.State != "needs_review" || !it.Downgraded {
		t.Errorf("cart.items = %+v", it)
	}
	if it := byKey["home.title"]; it.V0Status != "ai_translated" || it.State != "needs_review" || it.Downgraded {
		t.Errorf("home.title = %+v", it)
	}
	if tr := srv.translations["en"]["home.title"]; tr.origin != "import" {
		t.Errorf("provenance = %+v", tr)
	}

	// A reviewer approves one; a re-run changes nothing and keeps it approved.
	srv.translations["en"]["cart.items"].state = "approved"
	w.json(&out, args...).want(t, ExitOK)
	if out.Summary["message"]["unchanged"] != 2 || out.Summary["translation"]["unchanged"] != 2 || len(out.LocalesAdded) != 0 {
		t.Errorf("re-run = %+v", out.Summary)
	}
	if srv.translations["en"]["cart.items"].state != "approved" {
		t.Error("the re-run undid a review")
	}
}

func TestImportFromV03WithoutReviewKeepsApprovals(t *testing.T) {
	srv := newFakeServer(t)
	srv.sourceLocale, srv.locales, srv.reviewRequired = "de", []string{"de", "en"}, false
	old := fakeV03(t)
	w := newWorkspace(t).withProject(srv, nil)
	w.write("glossa.yaml", strings.Replace(w.read("glossa.yaml"), "source_locale: en", "source_locale: de", 1))
	w.env["GLOSSA_V0_KEY"] = "glossa_v03key"
	var out importJSON
	w.json(&out, "import", "--from", "v0", "--v0-url", old.URL+"/api/v1", "--v0-project", "site").want(t, ExitOK)
	if srv.translations["en"]["cart.items"].state != "approved" || srv.translations["en"]["home.title"].state != "needs_review" {
		t.Errorf("states = %s / %s", srv.translations["en"]["cart.items"].state, srv.translations["en"]["home.title"].state)
	}
}

func TestImportUsageErrors(t *testing.T) {
	srv := newFakeServer(t)
	w := newWorkspace(t).withProject(srv, nil)
	var doc errorDoc
	w.json(&doc, "import", "--from", "crowdin").want(t, ExitUsage)
	w.json(&doc, "import", "--from", "v0").want(t, ExitUsage)
	w.json(&doc, "import", "--from", "v0", "--v0-url", "http://x").want(t, ExitUsage)
	if doc.Error.Code != "no_v0_key" {
		t.Errorf("error = %+v", doc.Error)
	}
	old := fakeV03(t)
	w.env["GLOSSA_V0_KEY"] = "wrong"
	w.json(&doc, "import", "--from", "v0", "--v0-url", old.URL, "--v0-project", "site").want(t, ExitNetwork)
	if doc.Error.Code != "v0_unauthenticated" {
		t.Errorf("error = %+v", doc.Error)
	}
	w.env["GLOSSA_V0_KEY"] = "glossa_v03key"
	srv.sourceLocale = "fr"
	w.write("glossa.yaml", strings.Replace(w.read("glossa.yaml"), "source_locale: en", "source_locale: fr", 1))
	w.json(&doc, "import", "--from", "v0", "--v0-url", old.URL, "--v0-project", "site").want(t, ExitUsage)
	if doc.Error.Code != "source_locale_missing" || !strings.Contains(doc.Error.Why, "de, en") {
		t.Errorf("error = %+v", doc.Error)
	}
}
