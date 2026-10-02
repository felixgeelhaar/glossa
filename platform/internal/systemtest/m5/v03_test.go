//go:build system

package m5_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// ── §12.6 v0.3 imports and renders the same ──────────────────────────

// The v0.3 fixture: a real v0.3 server built from apps/api in this
// repository, on its own Postgres with its own migrations.
const (
	v03Tenant   = "acme-v03"
	v03Project  = "shop"
	v03Admin    = "admin@acme-v03.example"
	v03Password = "v03-admin-password-m5"
	v03JWTKey   = "m5-v03-jwt-signing-key-of-at-least-32-bytes"
	v03Keys     = 300
)

var v03Locales = []string{"de", "en", "es"}

// v03Users are the three users §12.6 seeds, with what they should become
// on the platform (§7.2: admin → admin, translator → translator with its
// locales). The first is the bootstrap admin.
var v03Users = []struct {
	Email, Role string
	Locales     []string
}{
	{v03Admin, "admin", nil},
	{"tomas@acme-v03.example", "translator", []string{"en"}},
	{"lucia@acme-v03.example", "translator", []string{"es"}},
}

type v03Report struct {
	Keys, Renderings int
	Mismatches       []renderRow
	Importer         string
	History          int
	Mode             string
}

// renderRow is one rendering of one key, locale and arguments, both ways.
type renderRow struct {
	Key          string         `json:"key"`
	Locale       string         `json:"locale"`
	Args         map[string]any `json:"args"`
	V0           *string        `json:"v0"`
	Runtime      *string        `json:"runtime"`
	V0Error      string         `json:"v0Error"`
	RuntimeError string         `json:"runtimeError"`
}

// v03Key is one seeded key: its text in each locale, its description,
// and the arguments the renderings use, generated from the arguments
// the key's ICU text declares.
type v03Key struct {
	Name        string
	Description string
	Text        map[string]string
	ArgSets     []map[string]any
}

// v03Catalog generates the 300 keys: plain text, simple arguments,
// plurals (with `=0` and `#`), selects, plurals nested in selects with a
// further argument, and apostrophes — escaped (`”`), literal (`l'été`)
// and quoting syntax (`'{'`).
func v03Catalog() []v03Key {
	var keys []v03Key
	add := func(name, desc string, text map[string]string, args ...map[string]any) {
		keys = append(keys, v03Key{Name: name, Description: desc, Text: text, ArgSets: args})
	}
	counts := func() []map[string]any {
		return []map[string]any{{"count": 0}, {"count": 1}, {"count": 2}, {"count": 5}, {"count": 21}}
	}
	for i := 1; len(keys) < v03Keys; i++ {
		n := fmt.Sprint(i)
		switch i % 6 {
		case 0:
			add("plain.label_"+n, "A plain label on screen "+n, map[string]string{
				"de": "Bestellung " + n + " ansehen", "en": "View order " + n, "es": "Ver pedido " + n,
			}, map[string]any{})
		case 1:
			add("greeting.name_"+n, "Greets the signed-in customer by name", map[string]string{
				"de": "Hallo {name}, willkommen zurück!", "en": "Hello {name}, welcome back!", "es": "¡Hola {name}, bienvenido de nuevo!",
			}, map[string]any{"name": "Ada"}, map[string]any{"name": "Zoë"})
		case 2:
			add("cart.items_"+n, "How many items are in the cart", map[string]string{
				"de": "{count, plural, =0 {Keine Artikel} one {# Artikel} other {# Artikel}}",
				"en": "{count, plural, =0 {No items} one {# item} other {# items}}",
				"es": "{count, plural, =0 {Ningún artículo} one {# artículo} other {# artículos}}",
			}, counts()...)
		case 3:
			add("activity.reply_"+n, "Who replied, by grammatical gender", map[string]string{
				"de": "{gender, select, female {Sie hat geantwortet} male {Er hat geantwortet} other {Sie haben geantwortet}}",
				"en": "{gender, select, female {She replied} male {He replied} other {They replied}}",
				"es": "{gender, select, female {Ella respondió} male {Él respondió} other {Respondieron}}",
			}, map[string]any{"gender": "female"}, map[string]any{"gender": "male"}, map[string]any{"gender": "other"})
		case 4:
			add("files.shared_"+n, "Who shared how many files, nested", map[string]string{
				"de": "{gender, select, female {{count, plural, one {{name} hat ihre # Datei geteilt} other {{name} hat ihre # Dateien geteilt}}} other {{count, plural, one {{name} hat # Datei geteilt} other {{name} hat # Dateien geteilt}}}}",
				"en": "{gender, select, female {{count, plural, one {{name} shared her # file} other {{name} shared her # files}}} other {{count, plural, one {{name} shared # file} other {{name} shared # files}}}}",
				"es": "{gender, select, female {{count, plural, one {{name} compartió su # archivo} other {{name} compartió sus # archivos}}} other {{count, plural, one {{name} compartió # archivo} other {{name} compartió # archivos}}}}",
			}, map[string]any{"gender": "female", "count": 1, "name": "Ada"}, map[string]any{"gender": "female", "count": 3, "name": "Ada"},
				map[string]any{"gender": "other", "count": 1, "name": "Sam"}, map[string]any{"gender": "other", "count": 7, "name": "Sam"})
		case 5:
			// Three kinds, one per key in turn: an escaped apostrophe
			// (`''`), a bare one in running text, and quoted syntax
			// characters. ICU and v0.3's formatter read a bare
			// apostrophe differently; real catalogs hold all three.
			switch (i / 6) % 3 {
			case 0:
				add("copy.escaped_"+n, "An escaped apostrophe", map[string]string{
					"de": "Das ist {name}s ''Wahl''", "en": "It''s {name}''s turn", "es": "Es el ''turno'' de {name}",
				}, map[string]any{"name": "Ada"})
			case 1:
				add("copy.bare_"+n, "An apostrophe in running text", map[string]string{
					"de": "Geht's gut, {name}?", "en": "Don't wait, {name}", "es": "Rock'n'roll con {name}",
				}, map[string]any{"name": "Ada"})
			case 2:
				add("copy.quoted_"+n, "Quoted braces", map[string]string{
					"de": "Nutze '{'Klammern'}', {name}", "en": "Use '{'braces'}', {name}", "es": "Usa '{'llaves'}', {name}",
				}, map[string]any{"name": "Ada"})
			}
		}
	}
	return keys
}

// v03Server is a running v0.3: its API, its database, and its seed.
type v03Server struct {
	api       string
	jwt       string
	apiKey    string
	container *tcpostgres.PostgresContainer
	dsn       func(db string) string
	audit     int
	bundles   map[string]map[string]string
}

func (s *scenario) migratable() {
	const id = "12.6"
	keys := v03Catalog()
	s.v03.Keys = len(keys)
	var srv *v03Server
	if !s.step(id, fmt.Sprintf("a v0.3 server built from apps/api, migrated with its own migrations, seeded: %d keys in de/en/es, descriptions, a change history, three users", len(keys)), func() error {
		var err error
		srv, err = startV03(s.t)
		if err != nil {
			return err
		}
		return srv.seed(keys)
	}) {
		s.unreached(id, "dump and restore it with the restore marker", "import it with `--v0-db`", "publish and render every key both ways",
			"descriptions, users and history carried", "the importer refuses a DSN without the marker")
		return
	}
	s.note(id, "v0.3 holds %d keys × %d locales, %d audit-log rows and %d users.", len(keys), len(v03Locales), srv.audit, len(v03Users))

	restored := ""
	s.step(id, "dump it and restore the dump with `platform/scripts/v0-restore.sh`, which writes the restore marker", func() error {
		var err error
		restored, err = srv.dumpAndRestore()
		return err
	})

	// The platform side: a project to import into, a token, and an
	// environment whose policy ships every imported state (an import by
	// a token cannot approve, §1.1), so the comparison renders every key.
	var p struct{ ID string }
	s.owner.do(http.MethodPost, s.tenantPath("/projects"),
		map[string]any{"slug": "shop-v03", "name": "Shop (from v0.3)", "source_locale": "de"}, http.StatusCreated, &p)
	for _, l := range []string{"en", "es"} {
		s.owner.do(http.MethodPost, s.projectPathOf(p.ID, "/locales"), map[string]string{"code": l}, http.StatusCreated, nil)
	}
	// The import sends v0.3's users as invitations (`--invite`, §7.2),
	// which takes a token that may invite: the owner's fixture token
	// (read, write, publish) may not, so the import runs on its own.
	var importer struct {
		Secret string `json:"secret"`
	}
	s.owner.do(http.MethodPost, s.tenantPath("/tokens"),
		map[string]any{"name": "m5-v03-import", "scopes": []string{"write", "admin"}}, http.StatusCreated, &importer)
	env := map[string]string{
		"GLOSSA_SERVER": s.d.base, "GLOSSA_TENANT": s.tenant, "GLOSSA_PROJECT": p.ID,
		"GLOSSA_TOKEN": importer.Secret, "GLOSSA_V0_KEY": srv.apiKey, "HOME": s.t.TempDir(),
	}
	dir := s.t.TempDir()
	// The CLI wants a project file; the environment above overrides its
	// server and project, as in CI.
	if err := os.WriteFile(filepath.Join(dir, "glossa.yaml"), []byte(fmt.Sprintf(
		"version: 1\nserver: %s\nproject: %s\nsource_locale: de\nsyntax: mf2\ncatalogs:\n  path: locales/{locale}.json\n",
		s.d.base, p.ID)), 0o644); err != nil {
		fatalf("%v", err)
	}

	imported := false
	if restored != "" {
		imported = s.step(id, "`glossa import --from v0 --v0-db` imports the restore", func() error {
			res := glossa(dir, env, append(cliImportV0DB, srv.dsn(restored), "--v0-project", v03Project, "--v0-tenant", v03Tenant,
				"--invite")...)
			if res.code != 0 {
				return fmt.Errorf("exit %d: %s", res.code, res.String())
			}
			return nil
		})
	} else {
		s.unreached(id, "`glossa import --from v0 --v0-db` imports the restore")
	}
	s.v03.Mode = "--v0-db"
	if !imported {
		// The comparison still runs, through the API importer that
		// exists since M2: it uses the same MF1 → MF2 converter and the
		// same runtime as `--v0-db` will, so a rendering difference
		// found here is one `--v0-db` would ship too. It is an early
		// signal, not the criterion's path: the criterion stays unmet
		// on the step above whatever this finds.
		s.v03.Mode = "the v0.3 API importer (early signal; `--v0-db` failed)"
		res := glossa(dir, env, "import", "--from", "v0", "--v0-url", srv.api, "--v0-project", v03Project)
		if res.code != 0 {
			s.gap(id, "even the API importer could not import the v0.3 project (exit %d: %s)", res.code, res.String())
			s.unreached(id, "publish and render every key both ways", "descriptions, users and history carried")
			s.markerRefusal(dir, env, srv)
			return
		}
	}

	s.step(id, fmt.Sprintf("publish, and render every key in every locale both ways: zero mismatches (imported by %s)", s.v03.Mode), func() error {
		return s.renderBothWays(p.ID, keys, srv)
	})
	if imported {
		s.carriedFields(p.ID, keys, srv)
	} else {
		s.unreached(id, "descriptions are on the messages", "the three users are invitations with mapped roles and locales",
			"v0.3's history is visible as imported audit entries")
	}
	s.markerRefusal(dir, env, srv)
}

// markerRefusal: the importer refuses v0.3's own database, which has no
// marker — so nobody points it at production v0.3.
func (s *scenario) markerRefusal(dir string, env map[string]string, srv *v03Server) {
	s.step("12.6", "the importer refuses a DSN without the restore marker", func() error {
		res := glossa(dir, env, append(cliImportV0DB, srv.dsn("v03"), "--v0-project", v03Project, "--v0-tenant", v03Tenant)...)
		out := strings.ToLower(res.stdout + res.stderr)
		switch {
		case res.code == 0:
			return fmt.Errorf("it imported from v0.3's own database")
		case !strings.Contains(out, "restore"):
			return fmt.Errorf("it exited %d, but not because the marker is missing: %s", res.code, res.String())
		}
		return nil
	})
}

func (s *scenario) renderBothWays(project string, keys []v03Key, srv *v03Server) error {
	s.owner.do(http.MethodPost, s.projectPathOf(project, "/environments"), map[string]any{
		"name": "v0-check", "policy": map[string]any{"states": []string{"draft", "needs_review", "approved"}, "include_outdated": true},
	}, http.StatusCreated, nil)
	var key struct {
		Key string `json:"key"`
	}
	s.owner.do(http.MethodPost, s.projectPathOf(project, "/delivery-keys"), map[string]any{
		"name": "v0-check", "scope": map[string]any{"environments": []string{"v0-check"}, "branches": false},
	}, http.StatusCreated, &key)
	rel, err := s.publish(s.owner, project, "v0-check", map[string]any{"note": "imported from v0.3"}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("publishing the import: %w", err)
	}
	if ok, served := s.edgeServes(key.Key, "v0-check", rel.ID, 20*time.Second); !ok {
		return fmt.Errorf("the edge never served the import's release (it served %s)", served)
	}
	var cases []map[string]any
	for _, k := range keys {
		for _, l := range v03Locales {
			for _, a := range k.ArgSets {
				cases = append(cases, map[string]any{"key": k.Name, "locale": l, "args": a})
			}
		}
	}
	in, _ := json.Marshal(map[string]any{
		"cases": cases, "v0": srv.bundles, "edgeURL": s.d.edgeURL, "deliveryKey": key.Key, "environment": "v0-check",
	})
	tmp := s.t.TempDir()
	inPath, outPath := filepath.Join(tmp, "in.json"), filepath.Join(tmp, "out.json")
	if err := os.WriteFile(inPath, in, 0o644); err != nil {
		return err
	}
	formatDir := filepath.Join(repoRoot(), "packages", "format")
	runtimeDir := filepath.Join(repoRoot(), "runtimes", "js", "runtime")
	for _, d := range []string{formatDir, runtimeDir} {
		if _, err := os.Stat(filepath.Join(d, "dist", "index.js")); err != nil {
			return fmt.Errorf("%s is not built: `make system-m5` builds it", strings.TrimPrefix(d, repoRoot()+"/"))
		}
	}
	here, _ := filepath.Abs(".")
	cmd := exec.Command("node", filepath.Join(here, "testdata", "render", "render.mjs"), inPath, outPath, formatDir, runtimeDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rendering failed: %v: %s", err, lastLines(string(out), 4))
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		return err
	}
	var res struct {
		Rows   []renderRow `json:"rows"`
		Errors []string    `json:"errors"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return err
	}
	s.v03.Renderings = len(res.Rows)
	for _, r := range res.Rows {
		if r.V0Error != "" || r.RuntimeError != "" || r.V0 == nil || r.Runtime == nil || *r.V0 != *r.Runtime {
			s.v03.Mismatches = append(s.v03.Mismatches, r)
		}
	}
	if len(res.Errors) > 0 {
		s.note("12.6", "The runtime reported: %s.", strings.Join(res.Errors, "; "))
	}
	if n := len(s.v03.Mismatches); n > 0 {
		m := s.v03.Mismatches[0]
		return fmt.Errorf("%d of %d renderings differ (first: `%s` %s %v — v0.3 %s, runtime %s)",
			n, len(res.Rows), m.Key, m.Locale, m.Args, quoteOr(m.V0, m.V0Error), quoteOr(m.Runtime, m.RuntimeError))
	}
	return nil
}

func quoteOr(s *string, err string) string {
	if err != "" {
		return "error: " + err
	}
	if s == nil {
		return "nothing"
	}
	return fmt.Sprintf("%q", *s)
}

// carriedFields: what `--v0-db` carries that the API importer cannot.
func (s *scenario) carriedFields(project string, keys []v03Key, srv *v03Server) {
	const id = "12.6"
	s.step(id, "descriptions are on the messages", func() error {
		items, err := list[struct {
			Key         string `json:"key"`
			Description string `json:"description"`
		}](s.owner, s.projectPathOf(project, "/messages"), nil)
		if err != nil {
			return err
		}
		want := map[string]string{}
		for _, k := range keys {
			want[k.Name] = k.Description
		}
		wrong := 0
		for _, it := range items {
			if d, ok := want[it.Key]; ok && it.Description != d {
				wrong++
			}
		}
		if wrong > 0 || len(items) == 0 {
			return fmt.Errorf("%d of %d messages lack v0.3's description", wrong, len(items))
		}
		return nil
	})
	s.step(id, "the three users are invitations with mapped roles and locales", func() error {
		members, err := list[struct {
			Email   string   `json:"email"`
			Status  string   `json:"status"`
			Roles   []string `json:"roles"`
			Locales []string `json:"locales"`
		}](s.owner, s.tenantPath("/members"), nil)
		if err != nil {
			return err
		}
		var missingUsers []string
		for _, u := range v03Users {
			found := false
			for _, m := range members {
				if strings.EqualFold(m.Email, u.Email) && m.Status == "invited" && contains(m.Roles, u.Role) && sameSet(m.Locales, u.Locales) {
					found = true
				}
			}
			if !found {
				missingUsers = append(missingUsers, fmt.Sprintf("%s (%s %v)", u.Email, u.Role, u.Locales))
			}
		}
		if len(missingUsers) > 0 {
			return fmt.Errorf("no matching invitation for %s", strings.Join(missingUsers, ", "))
		}
		return nil
	})
	s.step(id, "v0.3's history is visible as imported audit entries", func() error {
		entries, err := list[auditEntry](s.owner, s.auditEntriesPath(), url.Values{"action": {"v0.translation.changed"}})
		if err != nil {
			return missing("listing audit entries", err)
		}
		s.v03.History = len(entries)
		if len(entries) != srv.audit {
			return fmt.Errorf("%d imported audit entries, v0.3's audit log has %d rows", len(entries), srv.audit)
		}
		return nil
	})
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func sameSet(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, ",") == strings.Join(b, ",")
}

// ── the v0.3 server ──────────────────────────────────────────────────

func startV03(t interface {
	TempDir() string
	Cleanup(func())
}) (*v03Server, error) {
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("v03"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies())
	if err != nil {
		return nil, fmt.Errorf("start v0.3's Postgres: %w", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })
	base, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, err
	}
	srv := &v03Server{container: ctr, dsn: func(db string) string {
		u, _ := url.Parse(base)
		u.Path = "/" + db
		return u.String()
	}}

	// v0.3's own migrations, in order.
	conn, err := pgx.Connect(ctx, srv.dsn("v03"))
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	migrations, _ := filepath.Glob(filepath.Join(repoRoot(), "apps", "api", "db", "migrations", "*.up.sql"))
	sort.Strings(migrations)
	if len(migrations) == 0 {
		return nil, fmt.Errorf("apps/api has no migrations")
	}
	for _, m := range migrations {
		sql, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			return nil, fmt.Errorf("v0.3 migration %s: %w", filepath.Base(m), err)
		}
	}

	bin := filepath.Join(t.TempDir(), "v03-api")
	build := exec.Command("go", "build", "-o", bin, "./cmd/api")
	build.Dir = filepath.Join(repoRoot(), "apps", "api")
	if out, err := build.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build apps/api: %v: %s", err, lastLines(string(out), 4))
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"PORT="+port, "DATABASE_URL="+srv.dsn("v03"), "JWT_SIGNING_KEY="+v03JWTKey, "LOG_LEVEL=warn",
		"BOOTSTRAP_TENANT_SLUG="+v03Tenant, "BOOTSTRAP_TENANT_NAME=Acme (v0.3)",
		"BOOTSTRAP_ADMIN_EMAIL="+v03Admin, "BOOTSTRAP_ADMIN_PASSWORD="+v03Password)
	var logs syncBuffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	srv.api = "http://127.0.0.1:" + port
	deadline := time.Now().Add(60 * time.Second)
	for {
		if resp, err := http.Get(srv.api + "/readyz"); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the v0.3 server never became ready: %s", lastLines(logs.String(), 5))
		}
		time.Sleep(200 * time.Millisecond)
	}
	return srv, nil
}

// call performs a v0.3 API request with the admin's JWT.
func (v *v03Server) call(method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, v.api+"/api/v1"+path, r)
	req.Header.Set("Content-Type", "application/json")
	if v.jwt != "" {
		req.Header.Set("Authorization", "Bearer "+v.jwt)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("v0.3 %s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// seed fills v0.3 through its own API, the way its admin UI does.
func (v *v03Server) seed(keys []v03Key) error {
	var login struct {
		Token  string `json:"token"`
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	if err := v.call(http.MethodPost, "/auth/login", map[string]string{
		"tenantSlug": v03Tenant, "email": v03Admin, "password": v03Password,
	}, &login); err != nil {
		return err
	}
	v.jwt = login.Token
	var proj struct {
		APIKey string `json:"apiKey"`
	}
	if err := v.call(http.MethodPost, "/admin/projects", map[string]string{
		"tenantId": login.Tenant.ID, "slug": v03Project, "name": "Shop", "defaultLocale": "de",
	}, &proj); err != nil {
		return err
	}
	v.apiKey = proj.APIKey
	for _, l := range []struct{ code, label string }{{"en", "English"}, {"es", "Español"}} {
		if err := v.call(http.MethodPost, "/admin/projects/"+v03Project+"/locales", map[string]string{"code": l.code, "label": l.label}, nil); err != nil {
			return err
		}
	}
	rows := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, map[string]string{"name": k.Name, "description": k.Description})
	}
	if err := v.call(http.MethodPost, "/admin/projects/"+v03Project+"/keys:scan", map[string]any{"keys": rows}, nil); err != nil {
		return err
	}
	for _, l := range v03Locales {
		msgs := map[string]string{}
		for _, k := range keys {
			msgs[k.Name] = k.Text[l]
		}
		if err := v.call(http.MethodPost, "/admin/projects/"+v03Project+"/locales/"+l+"/bulk",
			map[string]any{"messages": msgs, "status": "approved"}, nil); err != nil {
			return err
		}
	}
	// A change history: every tenth key's English is edited, by the
	// admin, and v0.3 writes each edit to its audit_log.
	for i, k := range keys {
		if i%10 != 0 {
			continue
		}
		if err := v.call(http.MethodPatch, "/admin/projects/"+v03Project+"/locales/en/keys/"+k.Name,
			map[string]string{"value": k.Text["en"], "status": "approved", "updatedBy": v03Admin}, nil); err != nil {
			return err
		}
	}
	for _, u := range v03Users[1:] {
		if err := v.call(http.MethodPost, "/admin/users", map[string]any{
			"email": u.Email, "password": "translator-password-m5", "role": u.Role, "locales": u.Locales,
		}, nil); err != nil {
			return err
		}
	}
	var audit []json.RawMessage
	if err := v.call(http.MethodGet, "/admin/audit?limit=10000", nil, &audit); err != nil {
		return err
	}
	v.audit = len(audit)
	// What v0.3 serves its consumers: the bundles §12.6 renders the v0.3
	// side from.
	v.bundles = map[string]map[string]string{}
	jwt := v.jwt
	v.jwt = v.apiKey
	defer func() { v.jwt = jwt }()
	for _, l := range v03Locales {
		var b struct {
			Messages map[string]string `json:"messages"`
		}
		if err := v.call(http.MethodGet, "/projects/"+v03Project+"/locales/"+l+"/messages", nil, &b); err != nil {
			return err
		}
		v.bundles[l] = b.Messages
	}
	return nil
}

// dumpAndRestore takes a pg_dump of v0.3's database inside its
// container and restores it into a new database with the platform's
// restore script, which writes the marker.
func (v *v03Server) dumpAndRestore() (string, error) {
	script := filepath.Join(platformDir(), "scripts", "v0-restore.sh")
	marker := filepath.Join(platformDir(), "scripts", "v0-restore-marker.sql")
	for _, f := range []string{script, marker} {
		if _, err := os.Stat(f); err != nil {
			return "", fmt.Errorf("there is no restore script: %s does not exist", strings.TrimPrefix(f, repoRoot()+"/"))
		}
	}
	ctx := context.Background()
	for host, inside := range map[string]string{script: "/tmp/v0-restore.sh", marker: "/tmp/v0-restore-marker.sql"} {
		if err := v.container.CopyFileToContainer(ctx, host, inside, 0o755); err != nil {
			return "", err
		}
	}
	const db = "v03_restore"
	for _, c := range []string{
		"pg_dump -U postgres --no-owner v03 | gzip > /tmp/glossa-v03.sql.gz",
		"PGUSER=postgres GLOSSA_V0_MARKER_SQL=/tmp/v0-restore-marker.sql sh /tmp/v0-restore.sh /tmp/glossa-v03.sql.gz " + db,
	} {
		code, out, err := v.container.Exec(ctx, []string{"sh", "-c", c}, tcexec.Multiplexed())
		if err != nil {
			return "", err
		}
		if code != 0 {
			b, _ := io.ReadAll(out)
			return "", fmt.Errorf("`%s` exited %d: %s", c, code, lastLines(string(b), 4))
		}
	}
	return db, nil
}
