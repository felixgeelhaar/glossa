package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/integration/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
)

// The ports of the Glossa PR check (RFC 0004 §6.4): the queue the
// worker claims from, and the read model the report is rendered out of.

// CheckQueue is the check queue across tenants (system scope
// integration.github), the way DeliveryInbox is the webhook inbox. One
// row is one pull request, so claiming it is what keeps a single job
// per pull request — and therefore a single writer of the one sticky
// comment.
type CheckQueue interface {
	// Open records a pull request's check and makes it due now. An
	// existing row keeps its sticky comment and moves to the new head
	// SHA, which starts the wait for CI again.
	Open(ctx context.Context, c domain.Check) (domain.Check, error)
	// Rerun is `check_run.rerequested`: the check runs and the
	// annotations already sent to them are forgotten, because GitHub
	// makes new runs, and the row becomes due.
	Rerun(ctx context.Context, repository int64, headSHA string, now time.Time) (int, error)
	// Wake makes the branch's checks in these repositories due again,
	// because something the report mentions has changed.
	Wake(ctx context.Context, repositories []int64, branch string, now time.Time) (int, error)
	// Claim leases the oldest due check; ok is false when none is.
	Claim(ctx context.Context, lease time.Duration) (c domain.Check, ok bool, err error)
	// Save writes a claimed check back and releases the lease.
	Save(ctx context.Context, c domain.Check, available time.Time) error
	// Retry hands a claimed check back after delay, keeping what it
	// learned.
	Retry(ctx context.Context, c domain.Check, delay time.Duration, failure string) error
	// Expire makes every queued check whose head SHA was requested at or
	// before deadline due again; the worker completes them `neutral`.
	Expire(ctx context.Context, deadline, now time.Time) (int, error)
	// Depth reports the checks waiting to run (the §11 metric).
	Depth(ctx context.Context) (int, error)
	// DropRepository forgets a repository's checks.
	DropRepository(ctx context.Context, repository int64) (int, error)
	// Check reads one pull request's check.
	Check(ctx context.Context, repository int64, pullRequest int) (domain.Check, error)
}

// CheckFinding is one thing the check reports. A finding with a File
// and a Line becomes a GitHub annotation; the rest are summary only.
type CheckFinding struct {
	Code     string
	Severity checkpolicy.Severity
	Locale   string
	Key      string
	Message  string
	File     string
	Line     int
}

// Located reports whether the finding can become an annotation.
func (f CheckFinding) Located() bool { return f.File != "" && f.Line > 0 }

// InvalidMessage is one item the branch's last push could not accept.
type InvalidMessage struct{ Key, Code, Detail string }

// KeyConflict is a new key two open branches propose with different
// source (RFC 0004 §4.1).
type KeyConflict struct {
	Key      string
	Branches []string
}

// BranchStatus is Catalog's branch status as the check reports it —
// Integration's own shape, never Catalog's Go types.
type BranchStatus struct {
	Name       string
	PR         *int
	HeadCommit string
	State      string
	// PreviewURL is where CI deployed the branch's preview, if anywhere
	// (`glossa preview register --url`).
	PreviewURL      string
	NewKeys         []string
	SourceProposals []string
	Removed         []string
	Invalid         []InvalidMessage
	Conflicts       []KeyConflict
	// Outdated counts, per locale, the translations merging the branch
	// will make outdated.
	Outdated map[string]int
}

// BranchQuality is what Localization and Knowledge say about a branch:
// how far its new keys have got, per locale, and the QA findings on its
// messages.
type BranchQuality struct {
	// Locales are the project's target locales, in order.
	Locales []string
	// Untranslated counts, per locale, the branch's new keys with no
	// usable translation yet.
	Untranslated map[string]int
	// Findings are the QA warnings the server holds on the branch's
	// messages: max_length (Localization stores it with the text) and
	// terminology (M2).
	Findings []CheckFinding
}

// UnknownKey is a usage of a key the catalog did not know at ingest
// (RFC 0004 §2.2), with where the product asks for it.
type UnknownKey struct {
	Key  string
	File string
	Line int
}

// BranchUsages is what Context says about the branch's current builds.
type BranchUsages struct {
	// Builds counts the current builds the view resolved to; 0 means CI
	// has uploaded nothing for this branch.
	Builds int
	// Commits are those builds' commits. The check reads its own
	// readiness from them rather than from an event, so a build that is
	// ingested twice, late or never changes nothing but the answer.
	Commits []string
	Unknown []UnknownKey
	// Captured and NotCaptured count the branch's active messages with
	// and without a capture (RFC 0004 §3).
	Captured, NotCaptured int
}

// CheckSources is the read model the check renders from: the other
// bounded contexts' application services, reached through ports as
// RFC 0002 §4 requires, so each keeps checking the caller's
// permissions.
type CheckSources interface {
	// Policy is the project's check policy — the same `require_complete`
	// and `fail_on` as `glossa check` (RFC 0004 §6.4).
	Policy(ctx context.Context, project uuid.UUID) (checkpolicy.Policy, error)
	// BranchStatus is Catalog's branch report.
	BranchStatus(ctx context.Context, project uuid.UUID, branch string) (BranchStatus, error)
	// BranchQuality reports the branch's new keys per locale and the QA
	// findings on its messages. keys are the branch's own (new keys and
	// source proposals).
	BranchQuality(ctx context.Context, project uuid.UUID, branch string, keys []string) (BranchQuality, error)
	// BranchUsages reports the branch's unknown keys and capture counts.
	BranchUsages(ctx context.Context, project uuid.UUID, branch string) (BranchUsages, error)
	// ManifestURL is where the branch environment's manifest is served,
	// or "" when there is no environment or the deployment does not
	// announce its edge.
	ManifestURL(ctx context.Context, project uuid.UUID, branch string, pr int) (string, error)
}

// CheckMetrics records the check series of RFC 0004 §11. Every method
// must be safe for concurrent use and must not block.
type CheckMetrics interface {
	// CheckCompleted counts one completed check by conclusion, with the
	// latency from the pull-request event to the completed check.
	CheckCompleted(conclusion string, latency time.Duration)
	// CheckHandled counts one worker attempt by outcome ("done",
	// "waiting", "ignored", "retry", "failed") and how long it took.
	CheckHandled(outcome string, d time.Duration)
	// CommentUpserted counts one sticky-comment write by outcome ("ok",
	// "failed").
	CommentUpserted(outcome string)
	// QueueDepth reports the checks waiting to run.
	QueueDepth(n int)
}

// NoCheckMetrics records nothing.
type NoCheckMetrics struct{}

// CheckCompleted implements CheckMetrics.
func (NoCheckMetrics) CheckCompleted(string, time.Duration) {}

// CheckHandled implements CheckMetrics.
func (NoCheckMetrics) CheckHandled(string, time.Duration) {}

// CommentUpserted implements CheckMetrics.
func (NoCheckMetrics) CommentUpserted(string) {}

// QueueDepth implements CheckMetrics.
func (NoCheckMetrics) QueueDepth(int) {}
