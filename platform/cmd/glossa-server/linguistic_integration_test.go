//go:build integration

package main

import (
	"net/http"
	"testing"
)

// The linguistic-QA jobs API over HTTP (RFC 0005 §3.8, §9, wave 6):
// routing, permissions and what a deployment with no reviewer answers.
//
// The composition root wires the Linguist port with no Reviewer — the
// linguistic layer itself is the other wave-6 slice — so a review here
// is refused with `linguistic_unavailable` rather than answering a job
// that found nothing. That distinction is the point: "no model looked"
// and "a model looked and found nothing" are different statements, and
// only one of them is a clean bill of health. The service's own tests
// (internal/quality/app) drive the whole lifecycle against a fake
// reviewer; nothing anywhere calls a provider.
func TestLinguisticJobsAPIOverHTTP(t *testing.T) {
	s := startServer(t)
	ada := s.signIn("ada@example.com")
	var org struct{ ID string }
	s.do(call{method: "POST", path: "/v1/tenants", cookie: ada.cookie, csrf: ada.csrf,
		body: map[string]string{"slug": "acme", "name": "Acme"}}).decode(t, &org)
	base := "/v1/tenants/" + org.ID
	var project struct{ ID string }
	s.do(call{method: "POST", path: base + "/projects", cookie: ada.cookie, csrf: ada.csrf,
		body: map[string]any{"slug": "shop", "name": "Shop", "source_locale": "en"}}).decode(t, &project)
	p := base + "/projects/" + project.ID
	body := map[string]any{"ref": "main", "scope": map[string]any{"locales": []string{"de"}}}

	// Nothing has been asked for yet: an empty list, not a 404.
	var jobs struct{ Items []map[string]any }
	r := s.do(call{method: "GET", path: p + "/linguistic-jobs", cookie: ada.cookie})
	r.want(t, http.StatusOK, "")
	r.decode(t, &jobs)
	if len(jobs.Items) != 0 {
		t.Errorf("jobs of a project nobody reviewed = %s", r.body)
	}
	// A state outside the vocabulary is a 400, not a silent empty page.
	s.do(call{method: "GET", path: p + "/linguistic-jobs?state=pondering", cookie: ada.cookie}).
		want(t, http.StatusBadRequest, "invalid_query")
	// A job that isn't there is a 404, named or malformed.
	s.do(call{method: "GET", path: p + "/linguistic-jobs/0192f5a1-0000-7000-8000-000000000001", cookie: ada.cookie}).
		want(t, http.StatusNotFound, "not_found")
	s.do(call{method: "GET", path: p + "/linguistic-jobs/not-a-uuid", cookie: ada.cookie}).
		want(t, http.StatusNotFound, "not_found")

	// This deployment wires no reviewer, so a review says so.
	s.do(call{method: "POST", path: p + "/linguistic-jobs", cookie: ada.cookie, csrf: ada.csrf, body: body}).
		want(t, http.StatusServiceUnavailable, "linguistic_unavailable")

	// The session cookie alone cannot ask for one, and a read token may
	// look but not spend the tenant's AI budget.
	s.do(call{method: "POST", path: p + "/linguistic-jobs", cookie: ada.cookie, body: body}).
		want(t, http.StatusForbidden, "csrf_invalid")
	var readToken struct {
		Secret string `json:"secret"`
	}
	s.do(call{method: "POST", path: base + "/tokens", cookie: ada.cookie, csrf: ada.csrf,
		body: map[string]any{"name": "reader", "scopes": []string{"read"}}}).decode(t, &readToken)
	s.do(call{method: "GET", path: p + "/linguistic-jobs", bearer: readToken.Secret}).want(t, http.StatusOK, "")
	s.do(call{method: "POST", path: p + "/linguistic-jobs", bearer: readToken.Secret, body: body}).
		want(t, http.StatusForbidden, "forbidden")
	s.do(call{
		method: "POST", bearer: readToken.Secret,
		path: p + "/linguistic-jobs/0192f5a1-0000-7000-8000-000000000001/cancellation",
	}).want(t, http.StatusForbidden, "forbidden")
}
