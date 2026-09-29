package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// Errors. Adapters translate storage and Catalog errors into these.
var (
	// ErrNotFound is anything Quality holds that isn't there.
	ErrNotFound = errors.New("quality: not found")
	// ErrProjectNotFound is a project Catalog doesn't know.
	ErrProjectNotFound = errors.New("quality: no such project")
	// ErrCheckRunNotFound is a run that isn't this project's.
	ErrCheckRunNotFound = errors.New("quality: no such check run in the project")
	// ErrWaiverNotFound is a waiver that isn't this project's.
	ErrWaiverNotFound = errors.New("quality: no such waiver in the project")
	// ErrInvalidQuery means a list's filter is malformed.
	ErrInvalidQuery = errors.New("quality: invalid query")
	// ErrPreGradedFinding is a finding handed in at severity `waived`.
	// A run records what its layers found; whether a finding is waived
	// is decided from the project's waivers, here and again on read
	// (RFC 0005 §2.3), so a caller may not assert it.
	ErrPreGradedFinding = errors.New("quality: a recorded finding carries the severity its layer emitted, not `waived`")
	// ErrTooManyFindings is a run past domain.MaxRunFindings.
	ErrTooManyFindings = errors.New("quality: too many findings in one run")
	// ErrPolicyVersionNotFound is a policy version this project never
	// had.
	ErrPolicyVersionNotFound = errors.New("quality: no such check-policy version in the project")
	// ErrPolicyConflict is a policy write that lost a race: the project
	// moved between reading the document and replacing it, so saving
	// would drop whatever the other writer said. The caller reads the
	// policy again and decides.
	ErrPolicyConflict = errors.New("quality: the project's check policy changed while this write was being prepared")
)

// MaxRunFindings bounds one run's findings (RFC 0005 §10).
const MaxRunFindings = 10000

// Catalog is Catalog's application port, as Quality uses it: only
// whether a project exists, so an unknown one is a 404 and not an empty
// list. Quality keys its rows by Catalog's project, message and capture
// IDs and learns about them through ports (RFC 0002 §4).
type Catalog interface {
	// Project answers ErrProjectNotFound for an unknown project.
	Project(ctx context.Context, project uuid.UUID) error
	// CheckPolicy reads the project's stored check-policy document. A
	// project that has never saved one reads as the zero policy, which
	// is checkpolicy's documented default.
	CheckPolicy(ctx context.Context, project uuid.UUID) (StoredPolicy, error)
	// SaveCheckPolicy replaces the stored document, if the project is
	// still at the version the read saw (ErrPolicyConflict otherwise).
	SaveCheckPolicy(ctx context.Context, project uuid.UUID, ifMatch int, p checkpolicy.Policy) error
	// OpenPullRequests maps the names of the project's open branches to
	// the pull requests they belong to, for branches that have one. It
	// is what turns "these refs would newly fail" into the number
	// RFC 0005 §4.3 asks the impact preview to show: how many people
	// would wake up to a red pull request they did not cause.
	OpenPullRequests(ctx context.Context, project uuid.UUID) (map[string]int, error)
}

// StoredPolicy is the project's check policy as Catalog holds it.
type StoredPolicy struct {
	// Policy is the document that grades, history and all.
	Policy checkpolicy.Policy
	// ProjectVersion is the project row's version when the document was
	// read. A save hands it back, so a write that lost a race is refused
	// rather than silently overwriting the winner.
	ProjectVersion int
}

// Metrics records Quality in the deployment's metrics (RFC 0005 §11).
type Metrics interface {
	// FindingRecorded counts one finding of a run, by layer, code and
	// the severity it was stored with.
	FindingRecorded(layer domain.Layer, code string, severity domain.Severity)
	// CheckRunRecorded counts a run by what asked for it and what it
	// concluded.
	CheckRunRecorded(trigger domain.Trigger, conclusion domain.Conclusion)
	// WaiverDecided counts a waiver by what became of the request:
	// created, updated, revoked or refused.
	WaiverDecided(outcome string)
}

// Waiver outcomes, as Metrics counts them.
const (
	WaiverCreated = "created"
	WaiverUpdated = "updated"
	WaiverRevoked = "revoked"
	WaiverRefused = "refused"
)

// NoMetrics records nothing.
type NoMetrics struct{}

// FindingRecorded implements Metrics.
func (NoMetrics) FindingRecorded(domain.Layer, string, domain.Severity) {}

// CheckRunRecorded implements Metrics.
func (NoMetrics) CheckRunRecorded(domain.Trigger, domain.Conclusion) {}

// WaiverDecided implements Metrics.
func (NoMetrics) WaiverDecided(string) {}

// RunFilter narrows the check runs a list or a latest-run lookup sees.
// Empty members don't filter.
type RunFilter struct {
	// Ref is the branch or environment the run was of.
	Ref string
	// Commit is the commit it graded.
	Commit string
	// Conclusion is the verdict it reached; a run still in flight has
	// none and so matches no conclusion.
	Conclusion string
	// Trigger is what asked for it.
	Trigger string
}

// RunCursor is where a page of check runs (newest first) continues.
type RunCursor struct {
	StartedAt time.Time
	ID        uuid.UUID
}

// FindingFilter narrows a run's findings. Empty members don't filter;
// Waived is tri-state (nil: both).
type FindingFilter struct {
	Layer     string
	Severity  string
	Code      string
	Locale    string
	Namespace string
	// Key is the message key the finding is about, exactly.
	Key    string
	Waived *bool
}

// FindingRecord is a stored finding as it stands now: the finding the
// layer emitted, with the waiver that accepts it today applied.
type FindingRecord struct {
	domain.Finding
	// Waived says whether a live waiver covers it. The embedded
	// Finding's Severity and Waiver are already set from it.
	Waived bool
	// SortKey is the row's place in the run's stable order, and the page
	// cursor.
	SortKey string
}

// WaiverFilter narrows a project's waivers. The layer, code and key are
// the finding the waiver accepts, which a waiver names only by
// fingerprint: they are matched against the most recent finding
// carrying it. Active is tri-state (nil: both).
type WaiverFilter struct {
	Fingerprint string
	Layer       string
	Code        string
	Key         string
	Active      *bool
}

// WaiverCursor is where a page of waivers (newest first) continues.
type WaiverCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// FindingSummary is what the most recent stored finding carrying a
// fingerprint says about it. Its zero value means no stored finding
// carries that fingerprint any more.
type FindingSummary struct {
	Layer       string
	Code        string
	Locale      string
	Key         string
	Namespace   string
	Explanation string
	// SourceRevision is the source revision that finding was computed
	// against: what a waiver is made against when the caller names none.
	SourceRevision int
}

// WaiverRecord is a stored waiver with what it accepts. Accepts is zero
// where no stored finding carries the fingerprint any more — an
// unexamined waiver, which is exactly what a dashboard should show
// (RFC 0005 §2.3).
type WaiverRecord struct {
	domain.Waiver
	Accepts FindingSummary
	// Active says whether the waiver stands: neither revoked nor
	// expired, as of the read that produced the record.
	Active bool
}

// Transactor runs units of work scoped to the tenant on ctx.
type Transactor interface {
	InTenant(ctx context.Context, fn func(context.Context, Store) error) error
}

// Store is Quality's persistence port. Findings are immutable: they are
// inserted with their run and never updated (migration 0027).
type Store interface {
	// InsertCheckRun stores a run and its verdict.
	InsertCheckRun(ctx context.Context, r domain.CheckRun) error
	// InsertFindings stores a run's findings, at the severity their
	// layers emitted.
	InsertFindings(ctx context.Context, run, project uuid.UUID, fs []domain.Finding) error
	// CheckRun reads one of the project's runs (ErrCheckRunNotFound).
	CheckRun(ctx context.Context, project, id uuid.UUID) (domain.CheckRun, error)
	// LatestCheckRun is the newest run matching f (ErrCheckRunNotFound).
	LatestCheckRun(ctx context.Context, project uuid.UUID, f RunFilter) (domain.CheckRun, error)
	// ListCheckRuns pages a project's runs, newest first.
	ListCheckRuns(ctx context.Context, project uuid.UUID, f RunFilter, after *RunCursor, limit int) ([]domain.CheckRun, error)
	// ListFindings pages a run's findings, graded against the waivers
	// live at now and ordered stably.
	ListFindings(ctx context.Context, run domain.CheckRun, f FindingFilter, after string, limit int, now time.Time) ([]FindingRecord, error)
	// CountFindings sums a run's findings as they stand at now.
	CountFindings(ctx context.Context, run domain.CheckRun, now time.Time) (domain.Counts, error)
	// LatestFinding is the most recent stored finding carrying a
	// fingerprint; found is false where none does.
	LatestFinding(ctx context.Context, project uuid.UUID, fingerprint string) (summary FindingSummary, found bool, err error)
	// UpsertWaiver stores a waiver, or restates the project's live one
	// for the same fingerprint and reach; created says which happened.
	UpsertWaiver(ctx context.Context, w domain.Waiver) (stored domain.Waiver, created bool, err error)
	// Waiver reads one of the project's waivers (ErrWaiverNotFound).
	Waiver(ctx context.Context, project, id uuid.UUID) (domain.Waiver, error)
	// ListWaivers pages a project's waivers, newest first.
	ListWaivers(ctx context.Context, project uuid.UUID, f WaiverFilter, after *WaiverCursor, limit int, now time.Time) ([]WaiverRecord, error)
	// RevokeWaiver takes a waiver back; revoking an already revoked one
	// changes nothing and is not an error.
	RevokeWaiver(ctx context.Context, project, id uuid.UUID, at time.Time) error
	// LiveWaivers are the project's waivers that stand at now.
	LiveWaivers(ctx context.Context, project uuid.UUID, now time.Time) ([]domain.Waiver, error)
	// InsertPolicyVersion appends one saved policy version to the
	// project's history; stored is false where that version is already
	// recorded, which makes a repeated save idempotent rather than an
	// error.
	InsertPolicyVersion(ctx context.Context, v PolicyVersion) (stored bool, err error)
	// PolicyVersion reads one version of the project's policy
	// (ErrPolicyVersionNotFound).
	PolicyVersion(ctx context.Context, project uuid.UUID, version int) (PolicyVersion, error)
	// ListPolicyVersions pages the project's policy versions, newest
	// first, continuing below after.
	ListPolicyVersions(ctx context.Context, project uuid.UUID, after *int, limit int) ([]PolicyVersion, error)
}

// PolicyVersion is one saved version of a project's check policy: the
// document as it was stored, who wrote it and when (RFC 0005 §4.3).
//
// The document is the version itself, without the version behind it:
// the history is the sequence of these rows, not a chain inside one of
// them.
type PolicyVersion struct {
	ID      uuid.UUID
	Project uuid.UUID
	Version int
	Policy  checkpolicy.Policy
	// CreatedBy is the principal that saved it, and CreatedAt is when.
	// A policy is an organizational decision about a project, so who
	// took it is part of the record.
	CreatedBy string
	CreatedAt time.Time
}
