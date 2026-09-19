//go:build integration

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/objectstore/s3store/s3test"
)

type integrationJob struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	UploadURL   string `json:"upload_url"`
	DownloadURL string `json:"download_url"`
	FileName    string `json:"file_name"`
	FailureCode string `json:"failure_code"`
	ReusedJobID string `json:"reused_job_id"`
	Written     int    `json:"written"`
	File        struct {
		Size        int64  `json:"size"`
		Sha256      string `json:"sha256"`
		ContentType string `json:"content_type"`
	} `json:"file"`
	Summary struct {
		Created   int `json:"created"`
		Unchanged int `json:"unchanged"`
		Conflict  int `json:"conflict"`
	} `json:"summary"`
}

// raw sends a body as is (an upload) and returns the reply.
func (s *server) raw(method, path, bearer string, body []byte) reply {
	s.t.Helper()
	req, err := http.NewRequest(method, s.base+path, bytes.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	r := reply{status: resp.StatusCode, header: resp.Header}
	r.body, _ = io.ReadAll(resp.Body)
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json") {
		r.decode(s.t, &r.problem)
	}
	return r
}

// awaitJob polls a job until it has finished.
func (s *server) awaitJob(path, bearer string) integrationJob {
	s.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var j integrationJob
		s.do(call{method: "GET", path: path, bearer: bearer}).decode(s.t, &j)
		if j.State == "succeeded" || j.State == "failed" || j.State == "cancelled" {
			return j
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("job %s still %s", path, j.State)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Import and export end to end over HTTP, through the generated server,
// object storage (MinIO) and the in-process workers: create, upload,
// follow, page through results; export and download; the upload limit
// and the permission model at the edge.
func TestImportExportOverHTTP(t *testing.T) {
	ctx := context.Background()
	minio, err := s3test.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(minio.Close)
	if _, err := minio.Store(ctx, bucket); err != nil {
		t.Fatal(err)
	}
	vars := s3Vars(minio)
	vars["GLOSSA_INTEGRATION_MAX_UPLOAD_BYTES"] = "4096"
	vars["GLOSSA_INTEGRATION_POLL_INTERVAL"] = "50ms"
	s := startServerWith(t, vars)

	ada := s.signIn("ada@example.com")
	var org struct{ ID string }
	s.do(call{method: "POST", path: "/v1/tenants", cookie: ada.cookie, csrf: ada.csrf,
		body: map[string]string{"slug": "brotwerk", "name": "Brotwerk"}}).decode(t, &org)
	base := "/v1/tenants/" + org.ID
	var project struct{ ID string }
	s.do(call{method: "POST", path: base + "/projects", cookie: ada.cookie, csrf: ada.csrf,
		body: map[string]any{"slug": "web", "name": "Web", "source_locale": "en"}}).decode(t, &project)
	s.do(call{method: "POST", path: base + "/projects/" + project.ID + "/locales", cookie: ada.cookie, csrf: ada.csrf,
		body: map[string]string{"code": "de"}}).want(t, http.StatusCreated, "")
	token := func(scope string) string {
		var tok struct {
			Secret string `json:"secret"`
		}
		s.do(call{method: "POST", path: base + "/tokens", cookie: ada.cookie, csrf: ada.csrf,
			body: map[string]any{"name": "ci-" + scope, "scopes": []string{scope}}}).decode(t, &tok)
		return tok.Secret
	}
	ci, reader := token("write"), token("read")

	// A JSON source catalog, uploaded by CI.
	r := s.do(call{method: "POST", path: base + "/import-jobs", bearer: ci, headers: map[string]string{"Idempotency-Key": "push-1"},
		body: map[string]any{"project_id": project.ID, "format": "json", "file_name": "en.json"}})
	r.want(t, http.StatusCreated, "")
	var j integrationJob
	r.decode(t, &j)
	if j.State != "awaiting_upload" || j.UploadURL != base+"/import-jobs/"+j.ID+"/file" || r.header.Get("Location") == "" {
		t.Fatalf("created = %s", r.body)
	}
	s.raw("PUT", j.UploadURL, ci, []byte(`{"checkout": {"title": "Checkout"}, "home": {"title": "Welcome"}}`)).want(t, http.StatusOK, "")
	s.raw("PUT", j.UploadURL, ci, []byte(`{}`)).want(t, http.StatusConflict, "upload_not_expected")
	done := s.awaitJob(base+"/import-jobs/"+j.ID, ci)
	if done.State != "succeeded" || done.Summary.Created != 2 {
		t.Fatalf("import = %+v", done)
	}
	var results struct {
		Items []struct {
			Seq    int    `json:"seq"`
			Kind   string `json:"kind"`
			Key    string `json:"key"`
			Status string `json:"status"`
		} `json:"items"`
		NextPageToken string `json:"next_page_token"`
	}
	s.do(call{method: "GET", path: base + "/import-jobs/" + j.ID + "/results?page_size=1", bearer: reader}).decode(t, &results)
	if len(results.Items) != 1 || results.Items[0].Key != "checkout.title" || results.NextPageToken == "" {
		t.Errorf("results page = %+v", results)
	}
	s.do(call{method: "GET", path: base + "/import-jobs?project=" + project.ID, bearer: reader}).want(t, http.StatusOK, "")

	// A German XLIFF, with its translations.
	xlf := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.1" srcLang="en" trgLang="de"><file id="f1">
<unit id="checkout.title" name="checkout.title"><segment state="final"><source>Checkout</source><target>Kasse</target></segment></unit>
</file></xliff>`
	var xj integrationJob
	s.do(call{method: "POST", path: base + "/import-jobs", bearer: ci, body: map[string]any{"project_id": project.ID, "format": "xliff"}}).decode(t, &xj)
	s.raw("PUT", xj.UploadURL, ci, []byte(xlf)).want(t, http.StatusOK, "")
	if got := s.awaitJob(base+"/import-jobs/"+xj.ID, ci); got.State != "succeeded" || got.Summary.Created != 1 {
		t.Fatalf("xliff import = %+v", got)
	}

	// An export, downloaded.
	r = s.do(call{method: "POST", path: base + "/export-jobs", bearer: reader,
		body: map[string]any{"project_id": project.ID, "format": "json", "options": map[string]any{"locales": []string{"de"}, "states": []string{"approved", "needs_review"}}}})
	r.want(t, http.StatusCreated, "")
	var ej integrationJob
	r.decode(t, &ej)
	ej = s.awaitJob(base+"/export-jobs/"+ej.ID, reader)
	if ej.State != "succeeded" || ej.DownloadURL == "" || ej.Written != 2 || ej.FileName != "web.de.json" {
		t.Fatalf("export = %+v", ej)
	}
	dl := s.do(call{method: "GET", path: ej.DownloadURL, bearer: reader})
	dl.want(t, http.StatusOK, "")
	if string(dl.body) != "{\n  \"checkout.title\": \"Kasse\"\n}\n" || dl.header.Get("Content-Disposition") != `attachment; filename=web.de.json` ||
		dl.header.Get("ETag") != `"`+ej.File.Sha256+`"` || int64(len(dl.body)) != ej.File.Size {
		t.Errorf("download = %q %v", dl.body, dl.header)
	}

	// The edge: size limit, permissions, unknown jobs.
	var big integrationJob
	s.do(call{method: "POST", path: base + "/import-jobs", bearer: ci, body: map[string]any{"project_id": project.ID, "format": "json"}}).decode(t, &big)
	s.raw("PUT", big.UploadURL, ci, bytes.Repeat([]byte("x"), 5000)).want(t, http.StatusRequestEntityTooLarge, "file_too_large")
	s.do(call{method: "POST", path: base + "/import-jobs", bearer: reader, body: map[string]any{"project_id": project.ID, "format": "xliff"}}).
		want(t, http.StatusForbidden, "forbidden")
	s.do(call{method: "POST", path: base + "/export-jobs", bearer: reader, body: map[string]any{"format": "po"}}).
		want(t, http.StatusBadRequest, "invalid_format")
	s.do(call{method: "POST", path: base + "/import-jobs", bearer: ci, body: map[string]any{"format": "xliff"}}).
		want(t, http.StatusBadRequest, "project_required")
	s.do(call{method: "GET", path: base + "/export-jobs/" + j.ID, bearer: reader}).want(t, http.StatusNotFound, "not_found")
	r = s.do(call{method: "POST", path: base + "/import-jobs/" + big.ID + "/cancellation", bearer: ci})
	r.want(t, http.StatusOK, "")
	s.do(call{method: "POST", path: base + "/import-jobs/" + j.ID + "/cancellation", bearer: ci}).want(t, http.StatusConflict, "job_not_cancellable")
}
