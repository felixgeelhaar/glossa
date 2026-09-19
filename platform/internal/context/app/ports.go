// Package app is the Context context's application layer (RFC 0004 §2–
// §3): ingesting glossa.usages/v1 documents and captures, with keys
// resolved to message IDs at ingest through Catalog's port; the current
// usages of a message on the default branch or a branch view; the
// active messages no current build uses; and retention.
package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// Errors. Adapters translate storage and Catalog errors into these.
var (
	ErrNotFound            = errors.New("context: not found")
	ErrProjectNotFound     = errors.New("context: no such project")
	ErrApplicationNotFound = errors.New("context: no such application in the project")
	ErrMessageNotFound     = errors.New("context: no such message in the project")
	ErrBuildNotFound       = errors.New("context: no such build in the project")
	// ErrCaptureConflict means a build already holds a different
	// capture of the same route, viewport and locale.
	ErrCaptureConflict = errors.New("context: the build already holds another capture of this route, viewport and locale")
)

// MessageRef names a Catalog message.
type MessageRef struct {
	ID  uuid.UUID
	Key string
}

// Catalog is Catalog's application port, as Context uses it: projects,
// applications by slug, keys resolved to message IDs, the active
// messages, and (with the branch overlay, RFC 0004 §4.1) closed
// branches.
type Catalog interface {
	// Project answers ErrProjectNotFound for an unknown project.
	Project(ctx context.Context, project uuid.UUID) error
	// Application resolves an application's slug: ErrProjectNotFound or
	// ErrApplicationNotFound.
	Application(ctx context.Context, project uuid.UUID, slug string) (uuid.UUID, error)
	// MessageIDs resolves keys to message IDs; keys the catalog doesn't
	// know are absent from the map.
	MessageIDs(ctx context.Context, project uuid.UUID, keys []string) (map[string]uuid.UUID, error)
	// ActiveMessages lists the project's active messages in key order.
	ActiveMessages(ctx context.Context, project uuid.UUID) ([]MessageRef, error)
	// ClosedBranches reports when each of the project's closed branches
	// closed.
	ClosedBranches(ctx context.Context, project uuid.UUID) (map[domain.Branch]time.Time, error)
}

// Transactor runs units of work scoped to the tenant on ctx.
type Transactor interface {
	InTenant(ctx context.Context, fn func(context.Context, Store) error) error
}

// ProjectRef is a tenant's project, as the retention sweep finds it.
type ProjectRef struct {
	Tenant  tenancy.ID
	Project uuid.UUID
}

// Sweeper lists, across tenants (system scope), the projects holding
// builds: what the daily retention run visits.
type Sweeper interface {
	ProjectsWithBuilds(ctx context.Context) ([]ProjectRef, error)
}

// UsageView is a usage with the build it belongs to.
type UsageView struct {
	domain.Usage
	BuildID         uuid.UUID
	ApplicationID   uuid.UUID
	Commit          domain.Commit
	Branch          domain.Branch
	OnDefaultBranch bool
	Source          domain.Source
}

// Store is Context's persistence in tenant scope.
type Store interface {
	// InsertBuild stores b; inserted is false if a build of the same
	// upload (application, commit, source, digest) exists.
	InsertBuild(ctx context.Context, b domain.Build) (inserted bool, err error)
	// BuildByUpload returns the build of an upload (ErrNotFound).
	BuildByUpload(ctx context.Context, application uuid.UUID, commit domain.Commit, source domain.Source, digest domain.Digest) (domain.Build, error)
	Build(ctx context.Context, id uuid.UUID) (domain.Build, error)
	Builds(ctx context.Context, ids []uuid.UUID) ([]domain.Build, error)
	// BuildSummaries returns every build of the project.
	BuildSummaries(ctx context.Context, project uuid.UUID) ([]domain.BuildSummary, error)
	// InsertUsages stores a build's usages, by position.
	InsertUsages(ctx context.Context, build uuid.UUID, usages []domain.Usage) error
	CountUnknownKeys(ctx context.Context, build uuid.UUID) (int, error)
	// MessageUsages returns up to limit usages of message in builds.
	MessageUsages(ctx context.Context, message uuid.UUID, builds []uuid.UUID, limit int) ([]UsageView, error)
	// UsedMessages returns the messages with a usage in builds.
	UsedMessages(ctx context.Context, builds []uuid.UUID) ([]uuid.UUID, error)

	// LockBuildCaptures serializes adding captures to a build until the
	// transaction ends.
	LockBuildCaptures(ctx context.Context, build uuid.UUID) error
	CountCaptures(ctx context.Context, build uuid.UUID) (int, error)
	// InsertCapture stores c with its regions; inserted is false if the
	// build holds a capture of the same route, viewport and locale.
	InsertCapture(ctx context.Context, c domain.Capture) (inserted bool, err error)
	// CaptureByShot returns a build's capture of a route, viewport and
	// locale, with its regions (ErrNotFound).
	CaptureByShot(ctx context.Context, build uuid.UUID, route string, v domain.Viewport, locale bcp47.Tag) (domain.Capture, error)

	// BuildImages returns the images the builds' captures reference;
	// ReferencedImages which of digests the project's captures still do.
	BuildImages(ctx context.Context, builds []uuid.UUID) ([]domain.Digest, error)
	ReferencedImages(ctx context.Context, project uuid.UUID, digests []domain.Digest) ([]domain.Digest, error)
	// DeleteBuilds deletes builds with their usages, captures and
	// regions.
	DeleteBuilds(ctx context.Context, ids []uuid.UUID) (int, error)
	// DeleteProjectData erases everything Context holds for a project;
	// DeleteApplicationData for one of its applications.
	DeleteProjectData(ctx context.Context, project uuid.UUID) error
	DeleteApplicationData(ctx context.Context, project, application uuid.UUID) error

	Publish(ctx context.Context, e outbox.Event) error
}
