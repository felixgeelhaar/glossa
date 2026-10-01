//go:build integration

package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/v0/v0test"
)

// `glossa import --from v0 --v0-db` end to end on the CLI's side: a v0.3
// database from apps/api's migrations, backed up and restored with
// v0-restore.sh, imported into the fake platform.
func TestImportFromRestoredV03Database(t *testing.T) {
	ctx := context.Background()
	old, err := v0test.Start(ctx, v0test.Seed)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(old.Close)
	backup, err := old.Backup(ctx, "glossa-20261001")
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := old.Restore(ctx, backup, "v0_restore")
	if err != nil {
		t.Fatal(err)
	}

	srv := newFakeServer(t)
	srv.sourceLocale, srv.locales = "de", []string{"de"}
	w := newWorkspace(t).withProject(srv, nil)
	w.write("glossa.yaml", strings.Replace(w.read("glossa.yaml"), "source_locale: en", "source_locale: de", 1))
	args := []string{"import", "--from", "v0", "--v0-db", dsn, "--v0-tenant", "klarlabs", "--v0-project", "brotwerk"}

	var doc errorDoc
	w.json(&doc, "import", "--from", "v0", "--v0-db", old.DSN(v0test.LiveDB), "--v0-tenant", "klarlabs", "--v0-project", "brotwerk").
		want(t, ExitUsage)
	if doc.Error.Code != "v0_not_a_restore" || !strings.Contains(doc.Error.Fix, "v0-restore.sh") || srv.countRequests("POST") != 0 {
		t.Fatalf("v0.3's own database: error = %+v", doc.Error)
	}

	var dry importJSON
	w.json(&dry, append(args, "--dry-run")...).want(t, ExitOK)
	if dry.Summary["message"]["planned"] != 3 || dry.Summary["translation"]["planned"] != 3 || srv.countRequests("POST") != 0 {
		t.Fatalf("dry run = %+v", dry.Summary)
	}

	var out importJSON
	w.json(&out, args...).want(t, ExitOK)
	if out.Summary["message"]["created"] != 3 || out.Summary["translation"]["created"] != 3 || strings.Join(out.LocalesAdded, ",") != "en,fr" {
		t.Fatalf("import = %+v / %v", out.Summary, out.LocalesAdded)
	}
	if srv.messages["cart.items"].description != v0test.CartDesc || srv.messages["checkout.pay"].description != v0test.CheckoutDesc ||
		srv.messages["home.title"].description != "" {
		t.Errorf("descriptions: cart %q, checkout %q, home %q", srv.messages["cart.items"].description,
			srv.messages["checkout.pay"].description, srv.messages["home.title"].description)
	}
	cart := srv.translations["en"]["cart.items"]
	d := cart.originDetail
	if cart.origin != "import" || d["source"] != "glossa-v0.3" || d["tenant"] != "klarlabs" || d["restore"] != "glossa-20261001.sql.gz" ||
		d["v0_status"] != "approved" || d["v0_updated_by"] != "v0:"+v0test.BobID || d["v0_updated_at"] != "2025-02-02T10:30:00Z" ||
		d["v0_translation_id"] != v0test.TrEnCart {
		t.Errorf("en cart.items provenance = %s %v", cart.origin, d)
	}
	if d := srv.translations["en"]["home.title"].originDetail; d["v0_updated_by"] != "v0:ai:openai" || d["v0_status"] != "ai_translated" {
		t.Errorf("en home.title provenance = %v", d)
	}
	if d, ok := srv.translations["fr"]["home.title"].originDetail["v0_updated_by"]; !ok || d != nil {
		t.Errorf("fr home.title: v0_updated_by = %v (present %v); unknown must be an explicit null", d, ok)
	}

	if out.Restore == nil || out.Restore.Database != "v0_restore" || out.Source["db"] == "" || strings.Contains(out.Source["db"], "postgres:postgres") {
		t.Errorf("restore = %+v, source = %v", out.Restore, out.Source)
	}
	labels := map[string]string{}
	for _, l := range out.Locales {
		labels[l.Code] = l.Label
	}
	if labels["de"] != "Deutsch" || labels["en"] != "English (UK)" || labels["fr"] != "Français" {
		t.Errorf("locales = %+v", out.Locales)
	}
	status := map[string]string{}
	for _, i := range out.Invitations {
		status[i.Email] = i.Status + " " + strings.Join(i.Roles, ",") + " " + strings.Join(i.Locales, ",")
	}
	if status["alice@example.com"] != "planned admin " || status["bob@example.com"] != "planned translator en" ||
		!strings.HasPrefix(status["carol@example.com"], "held translator") || len(status) != 3 {
		t.Errorf("invitations = %v", status)
	}
	actors := map[string]int{}
	for _, e := range out.AuditEntries {
		actors[e.Actor]++
	}
	if len(out.AuditEntries) != 4 || actors["v0:"+v0test.BobID] != 2 || actors["v0:ai:openai"] != 1 || actors["v0:unknown"] != 1 {
		t.Errorf("audit entries = %+v", out.AuditEntries)
	}
	if len(out.NotCarried) == 0 || !containsWarning(out.Warnings, "fr") {
		t.Errorf("not carried = %+v, warnings = %v", out.NotCarried, out.Warnings)
	}

	human := w.run(append(args, "--dry-run")...)
	for _, want := range []string{"restore v0_restore of glossa-20261001.sql.gz", `en "English (UK)"`, `fr "Français" (disabled)`,
		"2 invitations planned, 1 held", "carol@example.com", "4 history entries planned", "1 without a translation", "users.password_hash"} {
		if !strings.Contains(human.stdout, want) {
			t.Errorf("human output lacks %q:\n%s", want, human.stdout)
		}
	}
	if strings.Contains(human.stdout+human.stderr, "postgres:postgres") {
		t.Error("human output leaks the DSN's credentials")
	}

	// A re-run changes nothing.
	w.json(&out, args...).want(t, ExitOK)
	if out.Summary["message"]["unchanged"] != 3 || out.Summary["translation"]["unchanged"] != 3 {
		t.Errorf("re-run = %+v", out.Summary)
	}
}

func containsWarning(ws []string, sub string) bool {
	for _, w := range ws {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}
