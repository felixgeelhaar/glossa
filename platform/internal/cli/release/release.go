// Package release is the Release context as the CLI sees it: publish,
// promote and roll back releases, list environments and delivery keys,
// and write a release's bundle for build-time catalogs (`glossa pull
// --release`, runtimes/SPEC.md §3.4).
//
// The /v1 API doesn't have the Release endpoints yet. Commands are
// written against Service; Unavailable implements it until an adapter
// over the generated client replaces it (a small follow-up once the
// endpoints are in api/openapi.yaml). WriteBundle is complete: it only
// needs a BundleSource.
package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// ErrUnavailable means the server has no Release API yet.
var ErrUnavailable = errors.New("release: the server's /v1 API has no Release endpoints yet")

// Scope is the tenant and project.
type Scope struct{ Tenant, Project string }

// Release is a published, immutable release.
type Release struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Note      string    `json:"note,omitempty"`
}

// Environment points at a release.
type Environment struct {
	Name    string   `json:"name"`
	Release *Release `json:"release,omitempty"`
}

// DeliveryKey is a publishable, read-only key for glossa-edge.
type DeliveryKey struct {
	ID          string `json:"id"`
	Prefix      string `json:"prefix"`
	Environment string `json:"environment"`
}

// PublishRequest publishes the project's eligible translations.
type PublishRequest struct {
	Environment string
	Note        string
}

// Service is what the release commands need from the server.
type Service interface {
	Environments(ctx context.Context, s Scope) ([]Environment, error)
	Publish(ctx context.Context, s Scope, r PublishRequest) (Release, error)
	Promote(ctx context.Context, s Scope, releaseID, environment string) (Environment, error)
	Rollback(ctx context.Context, s Scope, environment, toRelease string) (Environment, error)
	DeliveryKeys(ctx context.Context, s Scope) ([]DeliveryKey, error)
	BundleSource(s Scope, releaseID string) BundleSource
}

// Unavailable is the Service until the Release API exists.
type Unavailable struct{}

var _ Service = Unavailable{}

// Environments implements Service.
func (Unavailable) Environments(context.Context, Scope) ([]Environment, error) {
	return nil, ErrUnavailable
}

// Publish implements Service.
func (Unavailable) Publish(context.Context, Scope, PublishRequest) (Release, error) {
	return Release{}, ErrUnavailable
}

// Promote implements Service.
func (Unavailable) Promote(context.Context, Scope, string, string) (Environment, error) {
	return Environment{}, ErrUnavailable
}

// Rollback implements Service.
func (Unavailable) Rollback(context.Context, Scope, string, string) (Environment, error) {
	return Environment{}, ErrUnavailable
}

// DeliveryKeys implements Service.
func (Unavailable) DeliveryKeys(context.Context, Scope) ([]DeliveryKey, error) {
	return nil, ErrUnavailable
}

// BundleSource implements Service.
func (Unavailable) BundleSource(Scope, string) BundleSource { return unavailableSource{} }

type unavailableSource struct{}

func (unavailableSource) Manifest(context.Context) ([]byte, error) { return nil, ErrUnavailable }
func (unavailableSource) Artifact(context.Context, string) ([]byte, error) {
	return nil, ErrUnavailable
}

// ── bundles ─────────────────────────────────────────────────────────

// BundleSource serves one release's manifest and artifacts as exact
// bytes.
type BundleSource interface {
	Manifest(ctx context.Context) ([]byte, error)
	Artifact(ctx context.Context, sha256 string) ([]byte, error)
}

// manifest is the part of glossa.manifest/v1 a bundle needs.
type manifest struct {
	Schema  string `json:"schema"`
	Release struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	} `json:"release"`
	Artifacts map[string]map[string]struct {
		SHA256 string `json:"sha256"`
		Size   int    `json:"size"`
	} `json:"artifacts"`
}

// Bundle reports a written bundle.
type Bundle struct {
	Dir       string   `json:"dir"`
	ReleaseID string   `json:"release_id"`
	Version   int      `json:"version"`
	Locales   []string `json:"locales"`
	Artifacts int      `json:"artifacts"`
	Bytes     int      `json:"bytes"`
}

var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// WriteBundle writes the release to dir in the layout runtimes load
// (SPEC §3.4): manifest.json exactly as served, and a/<sha256>.json for
// every artifact, each verified against its manifest hash. The manifest
// is written last, so an interrupted run never leaves a manifest naming
// missing artifacts.
func WriteBundle(ctx context.Context, src BundleSource, dir string) (Bundle, error) {
	raw, err := src.Manifest(ctx)
	if err != nil {
		return Bundle{}, err
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Bundle{}, fmt.Errorf("release: manifest is not JSON: %w", err)
	}
	if m.Schema != "glossa.manifest/v1" {
		return Bundle{}, fmt.Errorf("release: manifest schema %q is not glossa.manifest/v1", m.Schema)
	}
	b := Bundle{Dir: dir, ReleaseID: m.Release.ID, Version: m.Release.Version, Bytes: len(raw)}
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
		return Bundle{}, err
	}
	written := map[string]bool{}
	for locale, namespaces := range m.Artifacts {
		b.Locales = append(b.Locales, locale)
		for ns, a := range namespaces {
			if !shaPattern.MatchString(a.SHA256) {
				return Bundle{}, fmt.Errorf("release: artifact %s/%s has an invalid sha256 %q", locale, ns, a.SHA256)
			}
			if written[a.SHA256] {
				continue
			}
			n, err := writeArtifact(ctx, src, dir, a.SHA256)
			if err != nil {
				return Bundle{}, fmt.Errorf("release: artifact %s/%s: %w", locale, ns, err)
			}
			written[a.SHA256] = true
			b.Artifacts++
			b.Bytes += n
		}
	}
	sort.Strings(b.Locales)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		return Bundle{}, err
	}
	return b, nil
}

func writeArtifact(ctx context.Context, src BundleSource, dir, sha string) (int, error) {
	data, err := src.Artifact(ctx, sha)
	if err != nil {
		return 0, err
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != sha {
		return 0, fmt.Errorf("integrity: bytes hash to %s, the manifest says %s", got, sha)
	}
	return len(data), os.WriteFile(filepath.Join(dir, "a", sha+".json"), data, 0o644)
}
