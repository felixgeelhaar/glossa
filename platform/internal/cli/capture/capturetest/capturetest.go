// Package capturetest serves the fixture app of the `glossa capture`
// integration tests (capture/testdata/app, built from
// runtimes/js/capture/src/testing/cli-fixture.ts) and skips a browser test
// where no Chrome is installed. Test-only.
package capturetest

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/capture"
)

// Login is what a page request carried of the fixture login.
type Login struct {
	Path, Header, Cookie string
}

// App is the fixture app on a local server.
type App struct {
	*httptest.Server
	mu     sync.Mutex
	logins []Login
}

// Logins are the fixture header (X-Fixture-User) and cookie (session)
// each page request carried.
func (a *App) Logins() []Login {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Login(nil), a.logins...)
}

// Dir is capture/testdata/app.
func Dir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "testdata", "app")
}

// NewApp serves the fixture app: app.js, and index.html for every other
// path (the query picks the locale and the manifest environment).
func NewApp(t testing.TB) *App {
	t.Helper()
	dir := Dir()
	a := &App{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app.js" {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			http.ServeFile(w, r, filepath.Join(dir, "app.js"))
			return
		}
		if r.URL.Path == "/favicon.ico" {
			http.NotFound(w, r)
			return
		}
		login := Login{Path: r.URL.Path, Header: r.Header.Get("X-Fixture-User")}
		if c, err := r.Cookie("session"); err == nil {
			login.Cookie = c.Value
		}
		a.mu.Lock()
		a.logins = append(a.logins, login)
		a.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	}))
	t.Cleanup(a.Close)
	return a
}

// SkipWithoutChrome skips the test when err is Chrome failing to start,
// except in CI (CI=true), where a browser test must run.
func SkipWithoutChrome(t testing.TB, err error) {
	t.Helper()
	var be *capture.BrowserError
	if errors.As(err, &be) && os.Getenv("CI") != "true" {
		t.Skipf("no Chrome here: %v", err)
	}
}
