// Package app is the Catalog context's application layer: the project,
// application and message use cases, authorization (package authz) on
// every one of them, and the ports other contexts use to read the
// catalog (source content for translation QA, the release snapshot).
package app

import (
	"context"
	"errors"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
)

// Store errors. Adapters translate their storage errors into these.
var (
	ErrNotFound     = errors.New("catalog: not found")
	ErrSlugTaken    = errors.New("catalog: slug taken")
	ErrKeyTaken     = errors.New("catalog: message key taken")
	ErrStaleVersion = errors.New("catalog: version changed")
)

// Application errors.
var (
	ErrPreconditionFailed = errors.New("catalog: resource changed since it was read")
	ErrIdempotencyReuse   = errors.New("catalog: idempotency key reused for a different request")
	ErrIdempotencyBusy    = errors.New("catalog: a request with this idempotency key is in progress")
	ErrTooManyItems       = errors.New("catalog: a batch holds 1 to 500 items")
	ErrCoverageFilter     = errors.New("catalog: missing_in and outdated_in can't be combined")
	ErrNoCoverage         = errors.New("catalog: translation coverage is not available")
)

// Transactor runs units of work scoped to the tenant on ctx. Calls
// don't nest.
type Transactor interface {
	InTenant(ctx context.Context, fn func(context.Context, Store) error) error
}

// NamespaceSummary is a namespace of a project with how many of its
// messages are active and obsolete.
type NamespaceSummary struct {
	Name     domain.Namespace
	Active   int
	Obsolete int
}

// MessageFilter narrows a message list.
type MessageFilter struct {
	Namespace *domain.Namespace
	State     *domain.MessageState
	// KeyPrefix matches keys starting with it ("checkout.").
	KeyPrefix string
}

// Store is Catalog's persistence in tenant scope. Row-level security,
// not these methods, keeps it inside the tenant.
type Store interface {
	// InsertProject stores p; inserted is false if p.ID already exists
	// (an idempotent retry).
	InsertProject(ctx context.Context, p domain.Project, by domain.Author) (inserted bool, err error)
	Project(ctx context.Context, id domain.ProjectID) (domain.Project, error)
	LockProject(ctx context.Context, id domain.ProjectID) (domain.Project, error)
	Projects(ctx context.Context, after domain.ProjectID, limit int) ([]domain.Project, error)
	// UpdateProject saves p if the stored version is still expected.
	UpdateProject(ctx context.Context, p domain.Project, expected int) error
	DeleteProject(ctx context.Context, id domain.ProjectID) error

	InsertApplication(ctx context.Context, a domain.Application, by domain.Author) (inserted bool, err error)
	Application(ctx context.Context, project domain.ProjectID, id domain.ApplicationID) (domain.Application, error)
	LockApplication(ctx context.Context, project domain.ProjectID, id domain.ApplicationID) (domain.Application, error)
	Applications(ctx context.Context, project domain.ProjectID, after domain.ApplicationID, limit int) ([]domain.Application, error)
	UpdateApplication(ctx context.Context, a domain.Application, expected int) error
	DeleteApplication(ctx context.Context, project domain.ProjectID, id domain.ApplicationID) error

	// InsertMessage stores m with its first source revision; inserted is
	// false if m.ID already exists.
	InsertMessage(ctx context.Context, m domain.Message, first domain.SourceRevision, by domain.Author) (inserted bool, err error)
	MessageByKey(ctx context.Context, project domain.ProjectID, key domain.MessageKey) (domain.Message, error)
	MessageByID(ctx context.Context, project domain.ProjectID, id domain.MessageID) (domain.Message, error)
	LockMessageByKey(ctx context.Context, project domain.ProjectID, key domain.MessageKey) (domain.Message, error)
	// LockMessagesByKeys locks the existing messages among keys, in key
	// order, and returns them by key.
	LockMessagesByKeys(ctx context.Context, project domain.ProjectID, keys []domain.MessageKey) (map[domain.MessageKey]domain.Message, error)
	MessagesByKeys(ctx context.Context, project domain.ProjectID, keys []domain.MessageKey) (map[domain.MessageKey]domain.Message, error)
	MessagesByIDs(ctx context.Context, project domain.ProjectID, ids []domain.MessageID) (map[domain.MessageID]domain.Message, error)
	// Messages lists by key, after the given key.
	Messages(ctx context.Context, project domain.ProjectID, f MessageFilter, after domain.MessageKey, limit int) ([]domain.Message, error)
	ActiveMessages(ctx context.Context, project domain.ProjectID) ([]domain.Message, error)
	// Namespaces lists the namespaces that hold messages, by name, after
	// the given one, with their message counts.
	Namespaces(ctx context.Context, project domain.ProjectID, after domain.Namespace, limit int) ([]NamespaceSummary, error)
	// UpdateMessage saves m if the stored version is still expected.
	UpdateMessage(ctx context.Context, m domain.Message, expected int) error
	// AppendSourceRevision adds to the append-only source log.
	AppendSourceRevision(ctx context.Context, r domain.SourceRevision) error
	// SourceRevisions lists newest first, below the given revision.
	SourceRevisions(ctx context.Context, id domain.MessageID, before, limit int) ([]domain.SourceRevision, error)
	SourceRevision(ctx context.Context, id domain.MessageID, revision int) (domain.SourceRevision, error)

	// InsertBranch stores b; inserted is false if the project already has
	// a branch of that name (a concurrent first push).
	InsertBranch(ctx context.Context, b domain.Branch, by domain.Author) (inserted bool, err error)
	Branch(ctx context.Context, project domain.ProjectID, name domain.BranchName) (domain.Branch, error)
	LockBranch(ctx context.Context, project domain.ProjectID, name domain.BranchName) (domain.Branch, error)
	BranchesByIDs(ctx context.Context, ids []domain.BranchID) (map[domain.BranchID]domain.Branch, error)
	// UpdateBranch saves b if the stored version is still expected.
	UpdateBranch(ctx context.Context, b domain.Branch, expected int) error

	// SaveProposal stores p, replacing the branch's proposal for its key.
	SaveProposal(ctx context.Context, p domain.Proposal) error
	DeleteProposal(ctx context.Context, branch domain.BranchID, key domain.MessageKey) error
	// BranchProposals lists a branch's proposals in key order.
	BranchProposals(ctx context.Context, branch domain.BranchID) ([]domain.Proposal, error)
	// ProposalsForMessages lists every branch's proposals for ids.
	ProposalsForMessages(ctx context.Context, ids []domain.MessageID) ([]domain.Proposal, error)
	// LockMessagesByIDs locks the project's messages among ids, in key
	// order.
	LockMessagesByIDs(ctx context.Context, project domain.ProjectID, ids []domain.MessageID) (map[domain.MessageID]domain.Message, error)
	// ActiveKeysExcept lists the project's active keys not among keys.
	ActiveKeysExcept(ctx context.Context, project domain.ProjectID, keys []domain.MessageKey) ([]domain.MessageKey, error)
	// LockOrphanedProposedMessages locks the tenant's proposed messages
	// that no open branch proposes.
	LockOrphanedProposedMessages(ctx context.Context) ([]domain.Message, error)

	Publish(ctx context.Context, e outbox.Event) error
}

// TranslationImpact is Localization's count, per locale, of the current
// (not outdated, not rejected) translations of some messages: the ones
// a change to their source will make outdated. The composition root
// wires Localization in.
type TranslationImpact interface {
	CurrentTranslations(ctx context.Context, project domain.ProjectID, ids []domain.MessageID) (map[string]int, error)
}

// Coverage is a translation status filter on the message list.
type Coverage string

// Coverage filters.
const (
	// CoverageMissing: no translation in the locale.
	CoverageMissing Coverage = "missing"
	// CoverageOutdated: translated against an older source revision.
	CoverageOutdated Coverage = "outdated"
)

// CoverageQuery asks which messages of a project have a coverage status
// in a locale.
type CoverageQuery struct {
	Project  domain.ProjectID
	Locale   string
	Status   Coverage
	Filter   MessageFilter
	AfterKey string
	Limit    int
}

// MessageProjection is told, inside the transaction that changes them,
// which messages a bulk upsert changed: Localization keeps its own
// projection of the catalog in step through it (an in-process adapter;
// Catalog never writes Localization's tables), so its missing_in and
// outdated_in listings agree with Catalog at commit instead of one
// outbox poll later. The composition root wires it; without one, the
// outbox alone keeps the projection current.
type MessageProjection interface {
	MessagesChanged(ctx context.Context, ms []domain.Message) error
}

// TranslationCoverage is Localization's answer to "which messages are
// missing or outdated in this locale", in key order. Catalog doesn't
// read translations itself (RFC 0002 §4: contexts never touch each
// other's tables); the composition root wires Localization in.
type TranslationCoverage interface {
	MessagesWithCoverage(ctx context.Context, q CoverageQuery) ([]domain.MessageID, error)
}
