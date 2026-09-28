package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// Errors. Adapters translate storage errors into these.
var (
	// ErrNotFound means the project has nothing stored to read: no check
	// run at all, or none for the ref asked for.
	ErrNotFound = errors.New("quality: not found")
	// ErrInvalidQuery means a filter is not one of the closed
	// vocabularies (layer, severity) or a limit is out of range.
	ErrInvalidQuery = errors.New("quality: invalid query")
)

// RunSummary is one stored check run without its findings: what was
// checked, against which policy version, and how it ended.
type RunSummary struct {
	ID            uuid.UUID
	Ref           string
	Trigger       domain.Trigger
	PolicyVersion int
	Layers        []domain.Layer
	Counts        domain.Counts
	// Conclusion is empty while the run is still in flight.
	Conclusion domain.Conclusion
	CreatedBy  string
	StartedAt  time.Time
	// CompletedAt is nil while the run is still in flight.
	CompletedAt *time.Time
}

// FindingFilter narrows a run's findings. An empty field is no filter.
type FindingFilter struct {
	Layer      domain.Layer
	Locale     string
	Severity   domain.Severity
	MessageKey string
	// WaivedOnly keeps only the findings a waiver accepted. It is
	// separate from Severity because "waived" is a rendering of a
	// finding, not a rank a layer emits (RFC 0005 §2.1).
	WaivedOnly bool
}

// StoredFinding is a finding as a run stored it: the row's id, which is
// the stable keyset a page continues from, and the finding itself. The
// fingerprint identifies a finding across runs; the row id identifies
// this one occurrence of it, which is what a cursor needs.
type StoredFinding struct {
	ID uuid.UUID
	domain.Finding
}

// Store is Quality's persistence in tenant scope. Row-level security,
// not these methods, keeps it inside the tenant.
type Store interface {
	// LatestRun returns the project's most recent run, of ref when ref
	// is not empty, and ErrNotFound when there is none. completedOnly
	// skips runs still in flight.
	LatestRun(ctx context.Context, project uuid.UUID, ref string, completedOnly bool) (RunSummary, error)
	// Findings lists a run's findings by id, after the given id ("" for
	// the first page), at most limit rows.
	Findings(ctx context.Context, run uuid.UUID, f FindingFilter, after string, limit int) ([]StoredFinding, error)
}

// Transactor runs a unit of work in the caller's tenant scope.
type Transactor interface {
	InTenant(ctx context.Context, fn func(context.Context, Store) error) error
}
