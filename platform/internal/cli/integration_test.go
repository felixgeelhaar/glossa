//go:build integration

package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/cli"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/credentials"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db/dbtest"
)

// testAuthSecret is base64 of 42 bytes.
const testAuthSecret = "YS10ZXN0LWF1dGgtc2VjcmV0LXRoYXQtaXMtbG9uZy1lbm91Z2gtMTIz"

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// server is a real glossa-server binary on a testcontainers Postgres.
type server struct {
	t    *testing.T
	base string
	logs *syncBuffer
}

func startServer(t *testing.T) *server {
	t.Helper()
	env, err := dbtest.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)

	bin := filepath.Join(t.TempDir(), "glossa-server")
	build := exec.Command("go", "build", "-o", bin, "./cmd/glossa-server")
	build.Dir = filepath.Join("..", "..") // the platform module
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build glossa-server: %v\n%s", err, out)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	logs := &syncBuffer{}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"DATABASE_URL="+env.AppDSN,
		"MIGRATION_DATABASE_URL="+env.OwnerDSN,
		"GLOSSA_MIGRATE=up",
		"GLOSSA_HTTP_ADDR="+addr,
		"GLOSSA_AUTH_SECRET="+testAuthSecret,
		"GLOSSA_MAIL_DRIVER=log",
		"GLOSSA_STUDIO_URL=https://studio.test",
		"GLOSSA_OUTBOX_POLL_INTERVAL=50ms",
	)
	cmd.Stdout, cmd.Stderr = logs, logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
		}
	})
	s := &server{t: t, base: "http://" + addr, logs: logs}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(s.base + "/readyz"); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return s
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("glossa-server never became ready:\n%s", logs)
	return nil
}

type call struct {
	method, path string
	body         any
	cookie, csrf string
}

func (s *server) do(c call, wantStatus int, out any) http.Header {
	s.t.Helper()
	var body io.Reader
	if c.body != nil {
		b, _ := json.Marshal(c.body)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(c.method, s.base+c.path, body)
	if err != nil {
		s.t.Fatal(err)
	}
	if c.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != "" {
		req.Header.Set("Cookie", "__Host-glossa_session="+c.cookie)
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		s.t.Fatalf("%s %s = %d, want %d: %s", c.method, c.path, resp.StatusCode, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			s.t.Fatalf("decode %s: %v", raw, err)
		}
	}
	return resp.Header
}

var mailedToken = regexp.MustCompile(`/auth/sign-in#token=([A-Za-z0-9_-]{43})`)

// signIn runs the magic-link flow and returns the session cookie and
// CSRF token.
func (s *server) signIn(email string) (string, string) {
	s.t.Helper()
	s.do(call{method: "POST", path: "/v1/auth/magic-links", body: map[string]string{"email": email}}, http.StatusAccepted, nil)
	var link []string
	for range 50 {
		if m := mailedToken.FindAllStringSubmatch(s.logs.String(), -1); len(m) > 0 {
			link = m[len(m)-1]
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if link == nil {
		s.t.Fatalf("no sign-in link in the log:\n%s", s.logs)
	}
	var session struct {
		CSRFToken string `json:"csrf_token"`
	}
	h := s.do(call{method: "POST", path: "/v1/auth/magic-link-redemptions", body: map[string]string{"token": link[1]}}, http.StatusOK, &session)
	cookie, _, _ := strings.Cut(strings.TrimPrefix(h.Get("Set-Cookie"), "__Host-glossa_session="), ";")
	return cookie, session.CSRFToken
}

// fakeV03 serves a Glossa v0.3 project (de source, en translations).
func fakeV03(t *testing.T) *httptest.Server {
	bundles := map[string]string{
		"de": `{"project":"brotwerk","locale":"de","messages":{"cart.items":"{count, plural, one {# Brot} other {# Brote}}","checkout.pay":"Bezahle {amount, number}","home.title":"Willkommen"},"statuses":{"cart.items":"approved","checkout.pay":"approved","home.title":"approved"}}`,
		"en": `{"project":"brotwerk","locale":"en","messages":{"cart.items":"{count, plural, one {# loaf} other {# loaves}}","checkout.pay":"Pay {amount, number}","home.title":"Welcome"},"statuses":{"cart.items":"approved","checkout.pay":"ai_translated","home.title":"pending"}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer glossa_v03" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch p := r.URL.Path; {
		case p == "/api/v1/projects/brotwerk/locales":
			_, _ = io.WriteString(w, `[{"code":"de"},{"code":"en"}]`)
		case strings.HasPrefix(p, "/api/v1/projects/brotwerk/locales/"):
			_, _ = io.WriteString(w, bundles[strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/projects/brotwerk/locales/"), "/messages")])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type runner struct {
	t     *testing.T
	dir   string
	env   map[string]string
	store credentials.Store
}

func (r runner) run(want cli.ExitCode, out any, args ...string) string {
	r.t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Main(context.Background(), append(args, "--json"), cli.Env{
		Stdout: &stdout, Stderr: &stderr, Dir: r.dir, Credentials: r.store, Version: "integration",
		Getenv: func(k string) string { return r.env[k] },
	})
	if code != int(want) {
		r.t.Fatalf("glossa %s: exit %d, want %d\n%s\n%s", strings.Join(args, " "), code, want, stdout.String(), stderr.String())
	}
	if out != nil {
		if err := json.Unmarshal(stdout.Bytes(), out); err != nil {
			r.t.Fatalf("glossa %s --json: %v\n%s", strings.Join(args, " "), err, stdout.String())
		}
	}
	return stdout.String()
}

// TestCLIAgainstGlossaServer is M1's CLI loop against the real server:
// init → push (ICU MF1) → check fails (en missing) → import the
// translations from Glossa v0.3 → check passes → generate. Re-running
// push and import changes nothing.
func TestCLIAgainstGlossaServer(t *testing.T) {
	s := startServer(t)
	cookie, csrf := s.signIn("ada@example.com")
	var org struct{ ID string }
	s.do(call{method: "POST", path: "/v1/tenants", cookie: cookie, csrf: csrf,
		body: map[string]string{"slug": "klarlabs", "name": "Klarlabs"}}, http.StatusCreated, &org)
	base := "/v1/tenants/" + org.ID
	var project struct{ ID string }
	s.do(call{method: "POST", path: base + "/projects", cookie: cookie, csrf: csrf,
		body: map[string]any{"slug": "brotwerk", "name": "Brotwerk", "source_locale": "de"}}, http.StatusCreated, &project)
	s.do(call{method: "POST", path: base + "/projects/" + project.ID + "/locales", cookie: cookie, csrf: csrf,
		body: map[string]string{"code": "en"}}, http.StatusCreated, nil)
	var tok struct{ Secret string }
	s.do(call{method: "POST", path: base + "/tokens", cookie: cookie, csrf: csrf,
		body: map[string]any{"name": "ci", "scopes": []string{"write"}}}, http.StatusCreated, &tok)

	dir := t.TempDir()
	r := runner{t: t, dir: dir, env: map[string]string{"GLOSSA_TOKEN": tok.Secret, "GLOSSA_V0_KEY": "glossa_v03"},
		store: &credentials.File{Path: filepath.Join(t.TempDir(), "credentials.json")}}

	// init reads the tenant and the source locale from the server.
	var initOut struct {
		Checked bool `json:"checked_with_server"`
		Config  struct {
			Tenant       string `json:"tenant"`
			SourceLocale string `json:"source_locale"`
		} `json:"config"`
	}
	r.run(cli.ExitOK, &initOut, "init", "--server", s.base, "--project", "brotwerk",
		"--typescript", "src/glossa/messages.ts", "--vue", "src/glossa/glossa-vue.ts", "--go", "internal/msg/messages.go")
	if !initOut.Checked || initOut.Config.Tenant != org.ID || initOut.Config.SourceLocale != "de" {
		t.Fatalf("init = %+v", initOut)
	}
	if err := os.MkdirAll(filepath.Join(dir, "locales"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "locales", "de.json"), []byte(`{
  "cart": {"items": "{count, plural, one {# Brot} other {# Brote}}"},
  "checkout.pay": "Bezahle {amount, number}",
  "home.title": "Willkommen"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var push struct {
		Summary map[string]int `json:"summary"`
	}
	r.run(cli.ExitOK, &push, "push")
	if push.Summary["created"] != 3 {
		t.Fatalf("push = %+v", push)
	}
	r.run(cli.ExitOK, &push, "push")
	if push.Summary["unchanged"] != 3 {
		t.Errorf("second push = %+v", push)
	}

	var check struct {
		Passed   bool `json:"passed"`
		Findings []struct {
			Code, Locale, Key string
		} `json:"findings"`
	}
	r.run(cli.ExitCheckFailed, &check, "check")
	missing := 0
	for _, f := range check.Findings {
		if f.Code == "missing-translation" && f.Locale == "en" {
			missing++
		}
	}
	if check.Passed || missing != 3 {
		t.Fatalf("check before import = %+v", check)
	}

	old := fakeV03(t)
	var imp struct {
		Summary map[string]map[string]int `json:"summary"`
		Items   []struct {
			Kind, Key, Status, State, V0Status string
			Downgraded                         bool
		} `json:"items"`
	}
	r.run(cli.ExitOK, &imp, "import", "--from", "v0", "--v0-url", old.URL, "--v0-project", "brotwerk")
	if imp.Summary["message"]["unchanged"] != 3 || imp.Summary["translation"]["created"] != 3 {
		t.Fatalf("import = %+v", imp.Summary)
	}
	states := map[string]string{}
	for _, it := range imp.Items {
		if it.Kind == "translation" {
			states[it.Key] = it.State
		}
	}
	// review_required: approved waits for a human; ai_translated needs review; pending is a draft.
	if states["cart.items"] != "needs_review" || states["checkout.pay"] != "needs_review" || states["home.title"] != "draft" {
		t.Errorf("imported states = %v", states)
	}
	r.run(cli.ExitOK, &imp, "import", "--from", "v0", "--v0-url", old.URL, "--v0-project", "brotwerk")
	if imp.Summary["translation"]["unchanged"] != 3 {
		t.Errorf("re-import = %+v", imp.Summary)
	}

	r.run(cli.ExitOK, &check, "check")
	if !check.Passed {
		t.Fatalf("check after import = %+v", check)
	}

	var status struct {
		Locales []struct {
			Code        string
			Translated  int
			NeedsReview int `json:"needs_review"`
			Draft       int
		} `json:"locales"`
	}
	r.run(cli.ExitOK, &status, "status")
	if len(status.Locales) != 2 || status.Locales[1].Code != "en" || status.Locales[1].Translated != 3 || status.Locales[1].Draft != 1 {
		t.Errorf("status = %+v", status)
	}

	var gen struct {
		Files []struct{ Path, Kind string } `json:"files"`
	}
	r.run(cli.ExitOK, &gen, "generate")
	if len(gen.Files) != 3 {
		t.Fatalf("generate = %+v", gen)
	}
	ts, err := os.ReadFile(filepath.Join(dir, "src", "glossa", "messages.ts"))
	if err != nil || !strings.Contains(string(ts), `"checkout.pay": { amount: number };`) {
		t.Errorf("messages.ts = %s (%v)", ts, err)
	}
	goSrc, err := os.ReadFile(filepath.Join(dir, "internal", "msg", "messages.go"))
	if err != nil || !strings.Contains(string(goSrc), "func (m Messages) CartItems(count float64") {
		t.Errorf("messages.go = %s (%v)", goSrc, err)
	}
	r.run(cli.ExitOK, nil, "generate", "--check")
	r.run(cli.ExitOK, nil, "diff", "--exit-code")
}
