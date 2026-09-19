// Package release is the Release context as the CLI sees it: publish,
// list, diff, promote and roll back releases, list environments, manage
// delivery keys, and write a release's bundle for build-time catalogs
// (`glossa pull --release`, runtimes/SPEC.md §3.4).
//
// Commands are written against Service; remote.ReleaseService implements
// it over the generated /v1 client. The types here are the CLI's own:
// their JSON tags are part of the `glossa.cli.release.*/v1` output
// schemas (cmd/glossa/README.md), so they change only additively.
package release

import (
	"context"
	"time"
)

// Scope is the tenant and project.
type Scope struct{ Tenant, Project string }

// Policy decides which translations ship to an environment.
type Policy struct {
	States          []string `json:"states"`
	IncludeOutdated bool     `json:"include_outdated"`
}

// LocaleCounts counts what a release ships in one locale.
type LocaleCounts struct {
	Messages int `json:"messages"`
	// Outdated translations shipped (the policy allowed them).
	Outdated int `json:"outdated"`
}

// Counts sizes a release.
type Counts struct {
	// Messages is the number of source messages.
	Messages  int `json:"messages"`
	Artifacts int `json:"artifacts"`
	// NewArtifacts is what the publish uploaded; the rest were stored.
	NewArtifacts int                     `json:"new_artifacts"`
	Bytes        int                     `json:"bytes"`
	Locales      map[string]LocaleCounts `json:"locales"`
}

// Release is a published, immutable release.
type Release struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	// Environment is the one it was published to.
	Environment string `json:"environment"`
	// ParentID is what that environment served before it.
	ParentID       string    `json:"parent_id,omitempty"`
	Note           string    `json:"note,omitempty"`
	Author         string    `json:"author"`
	CreatedAt      time.Time `json:"created_at"`
	SourceLocale   string    `json:"source_locale"`
	Locales        []string  `json:"locales"`
	ManifestDigest string    `json:"manifest_digest"`
	Policy         Policy    `json:"policy"`
	Counts         Counts    `json:"counts"`
}

// Environment points at the release it serves.
type Environment struct {
	Name string `json:"name"`
	// CurrentReleaseID is empty until something was published or
	// promoted to it.
	CurrentReleaseID string    `json:"current_release_id,omitempty"`
	Policy           Policy    `json:"policy"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// LocaleDiff lists the message IDs that changed in one locale.
type LocaleDiff struct {
	Locale  string   `json:"locale"`
	Added   []string `json:"added"`
	Changed []string `json:"changed"`
	Removed []string `json:"removed"`
}

// Diff is what changed between two releases.
type Diff struct {
	ReleaseID string `json:"release_id"`
	// BaseReleaseID is empty when the release had nothing to compare
	// with (everything is added).
	BaseReleaseID string       `json:"base_release_id,omitempty"`
	Locales       []LocaleDiff `json:"locales"`
}

// DeliveryKey is a publishable, read-only key runtimes fetch releases
// from glossa-edge with. It is public by design (it ships in browser
// bundles) and scoped to one project.
type DeliveryKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// PublishRequest publishes the project's eligible translations to an
// environment.
type PublishRequest struct {
	Environment string
	Note        string
	// IdempotencyKey makes a retry return the first release instead of
	// publishing again.
	IdempotencyKey string
}

// Published is the outcome of a publish.
type Published struct {
	Release Release
	// Replayed is true when the idempotency key had already published
	// it: nothing new was published.
	Replayed bool
}

// Problem is one reason the catalog can't be released.
type Problem struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
	Key    string `json:"key,omitempty"`
	Locale string `json:"locale,omitempty"`
}

// PreviewRelease is what a publish would build.
type PreviewRelease struct {
	SourceLocale   string   `json:"source_locale"`
	Locales        []string `json:"locales"`
	ManifestDigest string   `json:"manifest_digest"`
	// Counts.NewArtifacts is what the publish would upload.
	Counts Counts `json:"counts"`
}

// Preview is a publish's dry run: what it would ship to an environment,
// compared with what the environment serves. Nothing is stored.
type Preview struct {
	Environment string
	Policy      Policy
	// BaseReleaseID is what the environment serves now (empty: nothing).
	BaseReleaseID string
	Releasable    bool
	Problems      []Problem
	// Release is nil when the catalog isn't releasable.
	Release *PreviewRelease
	Changes []LocaleDiff
}

// Service is what the release commands need from the server.
type Service interface {
	Environments(ctx context.Context, s Scope) ([]Environment, error)
	Environment(ctx context.Context, s Scope, name string) (Environment, error)
	// Releases lists releases newest first; limit 0 lists them all.
	Releases(ctx context.Context, s Scope, limit int) ([]Release, error)
	Release(ctx context.Context, s Scope, id string) (Release, error)
	// Diff compares a release with base (empty: its parent).
	Diff(ctx context.Context, s Scope, id, base string) (Diff, error)
	Publish(ctx context.Context, s Scope, r PublishRequest) (Published, error)
	// PreviewPublish runs a publish's build for environment and stores
	// nothing.
	PreviewPublish(ctx context.Context, s Scope, environment string) (Preview, error)
	Promote(ctx context.Context, s Scope, releaseID, environment string) (Environment, error)
	// Rollback points environment back at toRelease (empty: the newest
	// release it served before the current one).
	Rollback(ctx context.Context, s Scope, environment, toRelease string) (Environment, error)
	DeliveryKeys(ctx context.Context, s Scope) ([]DeliveryKey, error)
	CreateDeliveryKey(ctx context.Context, s Scope, name, idempotencyKey string) (DeliveryKey, error)
	RevokeDeliveryKey(ctx context.Context, s Scope, id string) error
	// BundleSource serves a release's manifest as environment serves it,
	// and its artifacts.
	BundleSource(s Scope, releaseID, environment string) BundleSource
}
