//go:build integration

package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The Context context (RFC 0004) is composed into the server: deleting
// a project erases its builds through Context's outbox subscriber. It
// has no routes until the Context API slice (RFC 0004 §13, wave 2), so
// the build is seeded in SQL.
func TestContextIsComposed(t *testing.T) {
	s := startServer(t)
	ada := s.signIn("ada@example.com")
	var org struct{ ID string }
	s.do(call{method: "POST", path: "/v1/tenants", cookie: ada.cookie, csrf: ada.csrf,
		body: map[string]string{"slug": "acme", "name": "Acme"}}).decode(t, &org)
	base := "/v1/tenants/" + org.ID
	var project struct{ ID string }
	s.do(call{method: "POST", path: base + "/projects", cookie: ada.cookie, csrf: ada.csrf,
		body: map[string]any{"slug": "shop", "name": "Shop", "source_locale": "en"}}).decode(t, &project)
	var application struct{ ID string }
	s.do(call{method: "POST", path: base + "/projects/" + project.ID + "/applications", cookie: ada.cookie, csrf: ada.csrf,
		body: map[string]any{"slug": "web", "name": "Web", "platform": "web"}}).decode(t, &application)

	ctx := context.Background()
	if _, err := s.db.Super.Exec(ctx, `
		INSERT INTO context_builds (id, tenant_id, project_id, application_id, commit_sha, branch, on_default_branch,
		                            source, tool_name, digest, usage_count, created_by, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, 'abcdef1', 'main', true, 'plugin', '@glossa/unplugin', $4, 0, 'test', now())`,
		org.ID, project.ID, application.ID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}

	s.do(call{method: "DELETE", path: base + "/projects/" + project.ID, cookie: ada.cookie, csrf: ada.csrf}).
		want(t, http.StatusNoContent, "")
	deadline := time.Now().Add(10 * time.Second)
	for {
		var n int
		if err := s.db.Super.QueryRow(ctx, "SELECT count(*) FROM context_builds WHERE project_id = $1", project.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the deleted project's builds were never erased")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
