// Package app is the Release context's application layer: environments
// and their policies, publishing (build, upload, record, point), promote
// and rollback, diffs, delivery keys, and keeping object storage — the
// only thing glossa-edge reads — in step with the database.
//
// The database is the source of truth; storage is a projection of it.
// Every change commits first and then writes the objects it affects
// (an environment's manifest, a key's index object), both right away
// and again from an outbox subscriber, so a storage outage delays
// delivery but never loses a change. Writes are state-based — they read
// the current row under a lock and write what it says — so retries and
// reordering converge on the latest state.
package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
	"github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// Errors. Adapters translate storage errors into these.
var (
	ErrNotFound            = errors.New("release: not found")
	ErrEnvironmentExists   = errors.New("release: the project already has an environment with that name")
	ErrStaleVersion        = errors.New("release: version changed")
	ErrPreconditionFailed  = errors.New("release: resource changed since it was read")
	ErrIdempotencyReuse    = errors.New("release: Idempotency-Key was used for a different request")
	ErrStorage             = errors.New("release: artifact storage is unavailable")
	ErrReleaseNotInProject = errors.New("release: no such release in this project")
	ErrInvalidPageToken    = errors.New("release: page_token is not one this list issued")
)

// Source is what a release is built from: Catalog's releasable source
// and Localization's releasable translations, joined (RFC 0002 §4:
// through their application ports, never their tables).
type Source interface {
	// CheckProject returns ErrNotFound if the project doesn't exist.
	CheckProject(ctx context.Context, project uuid.UUID) error
	// Snapshot reads the project's active messages and its translations
	// in the review states given.
	Snapshot(ctx context.Context, project uuid.UUID, states []string) (domain.Snapshot, error)
	// BranchOverlay reads what branch adds to the main catalog: its
	// proposed messages and its source proposals (RFC 0004 §4.1). A
	// branch the catalog doesn't know yet proposes nothing.
	BranchOverlay(ctx context.Context, project uuid.UUID, branch string) (domain.Overlay, error)
}

// EnvironmentRef names an environment of a tenant's project.
type EnvironmentRef struct {
	Tenant      tenancy.ID
	Project     uuid.UUID
	Environment string
}

// KeyRef names a delivery key of a tenant's project.
type KeyRef struct {
	Tenant  tenancy.ID
	Project uuid.UUID
	Key     uuid.UUID
}

// Scanner finds Release's background work across tenants (system
// scope); the work itself runs in each tenant's scope.
type Scanner interface {
	// DuePublishRequests lists publish requests due at now.
	DuePublishRequests(ctx context.Context, now time.Time, limit int) ([]EnvironmentRef, error)
	// StaleKeyIndexes lists active keys whose index object was last
	// written in a format older than version.
	StaleKeyIndexes(ctx context.Context, version, limit int) ([]KeyRef, error)
}

// Transactor runs units of work scoped to the tenant on ctx.
type Transactor interface {
	InTenant(ctx context.Context, fn func(context.Context, Store) error) error
}

// Store is Release's persistence in tenant scope.
type Store interface {
	// InsertEnvironment adds e; false if one with its name exists.
	InsertEnvironment(ctx context.Context, e domain.Environment, by string) (bool, error)
	Environment(ctx context.Context, project uuid.UUID, name string, lock bool) (domain.Environment, error)
	Environments(ctx context.Context, project uuid.UUID, after string, limit int) ([]domain.Environment, error)
	// LockProjectEnvironments locks every environment of the project, in
	// name order: it serializes publishes (versions have no gaps).
	LockProjectEnvironments(ctx context.Context, project uuid.UUID) ([]domain.Environment, error)
	// UpdateEnvironment saves e if the stored version is expected
	// (ErrStaleVersion otherwise).
	UpdateEnvironment(ctx context.Context, e domain.Environment, expected int) error
	DeleteProjectEnvironments(ctx context.Context, project uuid.UUID) ([]string, error)
	// BranchEnvironment is the environment of branch (ErrNotFound if
	// none).
	BranchEnvironment(ctx context.Context, project uuid.UUID, branch string, lock bool) (domain.Environment, error)
	CountBranchEnvironments(ctx context.Context, project uuid.UUID) (int, error)
	// DeleteEnvironment removes an environment (its pending publish
	// request with it); its deployments stay as history.
	DeleteEnvironment(ctx context.Context, project uuid.UUID, name string) (bool, error)

	// PublishRequest locks an environment's pending publish request
	// (ErrNotFound if none).
	PublishRequest(ctx context.Context, project uuid.UUID, environment string) (domain.PublishRequest, error)
	SavePublishRequest(ctx context.Context, r domain.PublishRequest) error
	// DeletePublishRequest removes r if it is still the pending request
	// (its ID): a request made since stays.
	DeletePublishRequest(ctx context.Context, r domain.PublishRequest) error

	// InsertRelease records r; false if a release with its ID exists.
	InsertRelease(ctx context.Context, r domain.Release) (bool, error)
	Release(ctx context.Context, project, id uuid.UUID) (domain.Release, error)
	// Releases lists releases below beforeVersion, newest first.
	Releases(ctx context.Context, project uuid.UUID, beforeVersion, limit int) ([]domain.Release, error)
	MaxReleaseVersion(ctx context.Context, project uuid.UUID) (int, error)

	AppendDeployment(ctx context.Context, d domain.Deployment) error
	LastDeploymentNumber(ctx context.Context, project uuid.UUID, environment string) (int, error)
	// Deployments lists an environment's history below before, newest first.
	Deployments(ctx context.Context, project uuid.UUID, environment string, before, limit int) ([]domain.Deployment, error)
	// RollbackTarget is the newest release environment served that is
	// older than version (ErrNotFound if none).
	RollbackTarget(ctx context.Context, project uuid.UUID, environment string, version int) (domain.Release, error)
	ServedIn(ctx context.Context, project uuid.UUID, environment string, release uuid.UUID) (bool, error)

	// InsertDeliveryKey adds k; false if a key with its ID exists.
	InsertDeliveryKey(ctx context.Context, k domain.DeliveryKey) (bool, error)
	DeliveryKey(ctx context.Context, project, id uuid.UUID, lock bool) (domain.DeliveryKey, error)
	DeliveryKeys(ctx context.Context, project, after uuid.UUID, limit int) ([]domain.DeliveryKey, error)
	ActiveDeliveryKeys(ctx context.Context, project uuid.UUID) ([]domain.DeliveryKey, error)
	RevokeDeliveryKey(ctx context.Context, k domain.DeliveryKey) error
	// MarkKeyIndexed records the format the key's index object was last
	// written in.
	MarkKeyIndexed(ctx context.Context, id uuid.UUID, version int) error

	Publish(ctx context.Context, e outbox.Event) error
}
