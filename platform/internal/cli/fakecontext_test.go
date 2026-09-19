package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
)

// fakeContext is the Context API's usage uploads, faithful to the
// contract's shapes: POST …/context-builds?source=… with a
// glossa.usages/v1 body answers 201 with the build, or 200 with
// Idempotent-Replayed for the same document again.
type fakeContext struct {
	uploads []fakeUpload
	builds  map[string]map[string]any // application|commit|source|digest → build
}

type fakeUpload struct {
	source string
	doc    fakeUsagesDoc
}

type fakeUsagesDoc struct {
	Schema      string `json:"schema"`
	Application string `json:"application"`
	Commit      string `json:"commit"`
	Branch      string `json:"branch"`
	Tool        struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"tool"`
	Usages []struct {
		Key string `json:"key"`
	} `json:"usages"`
}

func newFakeContext() *fakeContext { return &fakeContext{builds: map[string]map[string]any{}} }

func (f *fakeServer) routeContext(mux *http.ServeMux, p string) {
	mux.HandleFunc("POST "+p+"/context-builds", f.uploadUsages)
}

func (f *fakeServer) uploadUsages(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if !slices.Contains([]string{"plugin", "extract", "runtime", "capture"}, source) {
		problemResp(w, 400, "invalid_source", "source must be plugin, extract, runtime or capture")
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		problemResp(w, 400, "invalid_request", "the body is JSON")
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var doc fakeUsagesDoc
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Schema != "glossa.usages/v1" || len(doc.Commit) != 40 || doc.Usages == nil {
		problemResp(w, 400, "invalid_usages", "context: invalid glossa.usages/v1 document")
		return
	}
	if doc.Application != "web" {
		problemResp(w, 400, "unknown_application", "the project has no application with that slug")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctx.uploads = append(f.ctx.uploads, fakeUpload{source: source, doc: doc})
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	key := doc.Application + "|" + doc.Commit + "|" + source + "|" + digest
	if b, ok := f.ctx.builds[key]; ok {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSONResp(w, 200, b)
		return
	}
	unknown := 0
	for _, u := range doc.Usages {
		if _, ok := f.messages[u.Key]; !ok {
			unknown++
		}
	}
	b := map[string]any{
		"id": fmt.Sprintf("bld_%d", len(f.ctx.builds)+1), "application_id": "app_web", "commit": doc.Commit, "branch": doc.Branch,
		"on_default_branch": doc.Branch == "main", "source": source, "tool": doc.Tool, "digest": digest,
		"usages": len(doc.Usages), "unknown_keys": unknown, "created_by": "token:ci", "created_at": "2026-09-19T12:00:00Z",
	}
	f.ctx.builds[key] = b
	writeJSONResp(w, 201, b)
}
