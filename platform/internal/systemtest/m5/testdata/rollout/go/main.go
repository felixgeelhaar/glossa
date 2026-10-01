// Command rollout is the Go side of RFC 0006 §12.4: one runtime client
// per installation id, each loading through a transport that serves the
// edge's real answers (fetched once, then from memory), and the release
// each one activated.
//
//	go run ./internal/systemtest/m5/testdata/rollout/go <input.json> <output.json>
//
// It lives under testdata so `go build ./...` and `go vet ./...` never
// compile it: it is written against the rollout surface SPEC §1.4 adds
// to the Go runtime — an installation id in Config, and a switch that
// turns rollout support off — and until wave 2 builds that, it does not
// compile. That compile error is §12.4's reason for the Go runtime, in
// the report. The slice that implements rollout in the Go runtime makes
// the field names here agree with its own.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	glossa "github.com/felixgeelhaar/glossa/runtimes/go"
)

type input struct {
	EdgeURL     string   `json:"edgeURL"`
	DeliveryKey string   `json:"deliveryKey"`
	Environment string   `json:"environment"`
	IDs         []string `json:"ids"`
	Rollout     bool     `json:"rollout"`
	Manifest    string   `json:"manifest"`
}

type cached struct {
	status int
	header http.Header
	body   []byte
}

// edgeCache answers every request with the edge's answer to the first
// identical one, or with the probe manifest when there is one.
type edgeCache struct {
	mu       sync.Mutex
	seen     map[string]cached
	manifest string
}

func (c *edgeCache) RoundTrip(r *http.Request) (*http.Response, error) {
	u := r.URL.String()
	if c.manifest != "" && strings.HasSuffix(u, "/manifest.json") {
		return reply(r, cached{status: 200, header: http.Header{"Etag": {`"m5-probe"`}}, body: []byte(c.manifest)}), nil
	}
	c.mu.Lock()
	hit, ok := c.seen[u]
	c.mu.Unlock()
	if !ok {
		resp, err := http.Get(u)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		hit = cached{status: resp.StatusCode, header: resp.Header.Clone(), body: body}
		c.mu.Lock()
		c.seen[u] = hit
		c.mu.Unlock()
	}
	return reply(r, hit), nil
}

func reply(r *http.Request, c cached) *http.Response {
	return &http.Response{StatusCode: c.status, Header: c.header.Clone(), Body: io.NopCloser(bytes.NewReader(c.body)), Request: r}
}

func main() {
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(inPath, outPath string) error {
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}
	transport := &edgeCache{seen: map[string]cached{}, manifest: in.Manifest}
	out := map[string]*string{}
	for _, id := range in.IDs {
		c, err := glossa.New(glossa.Config{
			EdgeURL: in.EdgeURL, DeliveryKey: in.DeliveryKey, Environment: in.Environment,
			HTTPClient: &http.Client{Transport: transport}, DisableCache: true, RefreshInterval: -1,
			Retry: glossa.RetryPolicy{MaxAttempts: 1}, Logger: slog.New(slog.DiscardHandler),
			InstallationID: id, DisableRollout: !in.Rollout,
		})
		if err != nil {
			return err
		}
		_ = c.Refresh(context.Background())
		if rel, ok := c.Release(); ok {
			v := rel.ID
			out[id] = &v
		} else {
			out[id] = nil
		}
		_ = c.Close()
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, b, 0o644)
}
