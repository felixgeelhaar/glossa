package problem_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
)

func TestWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	problem.Write(rec, http.StatusRequestEntityTooLarge, "body exceeds 1024 bytes")

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != problem.ContentType {
		t.Errorf("Content-Type = %q", ct)
	}
	var got problem.Details
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := problem.Details{Title: "Request Entity Too Large", Status: 413, Detail: "body exceeds 1024 bytes"}
	if got != want {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}
