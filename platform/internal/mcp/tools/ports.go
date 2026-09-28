package tools

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/mcp/domain"
)

// ErrNotFound is what every port answers for something this tenant
// cannot see, whether it does not exist at all or belongs to another
// tenant. The two are deliberately the same answer (see the package
// doc): an adapter translates its context's own not-found into this and
// never distinguishes the cases.
var ErrNotFound = domain.ErrNotFound

// ── Catalog ─────────────────────────────────────────────────────────

// CatalogFilter narrows catalog_search. Every field is optional and an
// empty one is no filter.
type CatalogFilter struct {
	KeyPrefix  string
	Namespace  string
	State      string
	MissingIn  string
	OutdatedIn string
	// Unused selects the active messages no current build refers to
	// (RFC 0004 §2.2). It is its own mode: Context answers it, and only
	// KeyPrefix narrows it further.
	Unused bool
}

// MessageSummary is one message as a search returns it: enough to
// decide whether it is the message the agent meant, and the id and key
// to ask anything further.
//
// Omitted: arguments and markup (derived from Source, and message_get
// returns them), description, max_length, version and timestamps. A
// search result is a shortlist, not a record.
type MessageSummary struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Namespace string `json:"namespace,omitempty"`
	State     string `json:"state"`
	// Source is the current source text in its own syntax, and Revision
	// the source revision it is.
	Source   string `json:"source"`
	Syntax   string `json:"syntax"`
	Revision int    `json:"revision"`
}

// MessageDetail is one message in full, as message_get returns it.
//
// Omitted: the source revision history (a message can have hundreds),
// version and timestamps, and the message's translations — one locale
// at a time through translation_get, because a catalog with forty
// locales would otherwise answer a single call with forty texts.
type MessageDetail struct {
	MessageSummary
	Description string `json:"description,omitempty"`
	MaxLength   *int   `json:"max_length,omitempty"`
	// Arguments are the placeholders the source declares, by name.
	Arguments []string `json:"arguments,omitempty"`
}

// Catalog is Catalog's application service, as MCP reads it.
type Catalog interface {
	// SearchMessages lists a project's messages in key order. after is
	// the keyset of the previous page ("" for the first) and limit
	// bounds the rows; it returns the cursor for the next page, or "".
	SearchMessages(
		ctx context.Context, project uuid.UUID, f CatalogFilter, after string, limit int,
	) (msgs []MessageSummary, next string, err error)
	// Message returns one message by key, ErrNotFound when this tenant
	// has no such message in no such project.
	Message(ctx context.Context, project uuid.UUID, key string) (MessageDetail, error)
}

// ── Localization ────────────────────────────────────────────────────

// Translation is one locale's text for one message.
//
// Omitted: the revision log (its own resource), the translation's own
// id — a translation is addressed by message key and locale everywhere
// — and the structural QA warnings' detail, which findings_list reports
// properly; only their codes are kept, as a hint that there is
// something to look at.
type Translation struct {
	MessageID string `json:"message_id"`
	Key       string `json:"key"`
	Locale    string `json:"locale"`
	Text      string `json:"text"`
	Syntax    string `json:"syntax"`
	// State is the review state and Origin the provenance of the current
	// text ("human", "ai", "translation-memory", …), By who wrote it.
	State  string `json:"state"`
	Origin string `json:"origin"`
	By     string `json:"by,omitempty"`
	// SourceRevision is the source this text was made against;
	// CurrentSourceRevision the source that ships now. Outdated is the
	// two disagreeing.
	SourceRevision        int      `json:"source_revision"`
	CurrentSourceRevision int      `json:"current_source_revision"`
	Outdated              bool     `json:"outdated"`
	Revision              int      `json:"revision"`
	Warnings              []string `json:"warnings,omitempty"`
}

// Translations is Localization's application service, as MCP reads it.
type Translations interface {
	// Translation returns a message's text in one locale, ErrNotFound
	// when the project, the message or the translation is not this
	// tenant's to see.
	Translation(ctx context.Context, project uuid.UUID, key, locale string) (Translation, error)
}

// ── Context ─────────────────────────────────────────────────────────

// Usage is one place in the product where a message is asked for.
//
// Omitted: the build, application and commit ids, the usage's position
// in its document, and the source tool that reported it. An agent wants
// the file and the line; provenance of the upload is the API's.
type Usage struct {
	File      string `json:"file"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	Component string `json:"component,omitempty"`
	Route     string `json:"route,omitempty"`
	// Kind is the call shape the collector recognized (t, component,
	// element, accessor, template).
	Kind string `json:"kind,omitempty"`
	// Branch is the branch the build came from, and OnDefaultBranch says
	// whether that is the project's default branch.
	Branch          string `json:"branch,omitempty"`
	OnDefaultBranch bool   `json:"on_default_branch"`
}

// Usages is a message's usages, up to the limit asked for.
type Usages struct {
	MessageID string  `json:"message_id"`
	Key       string  `json:"key"`
	Branch    string  `json:"branch,omitempty"`
	Usages    []Usage `json:"usages"`
	// Truncated says more usages exist than were returned. Context's
	// query is a bounded read and issues no cursor, so this is how a
	// caller learns it has not seen everything.
	Truncated bool `json:"truncated"`
}

// Neighbour is a message shown together with another one.
type Neighbour struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// UsageReader is the Context context, as MCP reads it.
type UsageReader interface {
	// OfKey returns the current usages of the message a key names,
	// ErrNotFound when there is no such message.
	OfKey(ctx context.Context, project uuid.UUID, key, branch string, limit int) (Usages, error)
	// CaptureCount counts the current captures that show the message.
	CaptureCount(ctx context.Context, project uuid.UUID, key string, limit int) (n int, truncated bool, err error)
	// Neighbours lists messages shown together with this one.
	Neighbours(ctx context.Context, project uuid.UUID, message uuid.UUID, limit int) ([]Neighbour, error)
}

// ── Knowledge ───────────────────────────────────────────────────────

// TMMatch is one translation-memory match.
//
// Omitted: the unit's normalized forms, its data model, its hit count
// and timestamps, and the translation it was derived from. What decides
// whether to reuse a match is the score, the text and where it came
// from.
type TMMatch struct {
	Unit   string `json:"unit"`
	Score  int    `json:"score"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Target string `json:"target"`
	Syntax string `json:"syntax"`
	// MessageKey and Namespace are the message the unit was derived
	// from, which is what makes a 101 an in-context match.
	MessageKey string `json:"message_key,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	// Adapted says the target's variables were renamed to the query's.
	Adapted bool `json:"adapted"`
}

// TermUse is a term of a concept in the target locale.
type TermUse struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

// Term is one termbase term recognized in a text.
//
// Omitted: the concept's version, its author and timestamps, its
// product reference and its terms in every other locale.
type Term struct {
	Concept string `json:"concept"`
	Text    string `json:"text"`
	Status  string `json:"status"`
	// Start and End delimit the match in the text sent, in bytes.
	Start int `json:"start"`
	End   int `json:"end"`
	// Definition is the concept's definition, Domain its subject area.
	Definition string `json:"definition,omitempty"`
	Domain     string `json:"domain,omitempty"`
	// Targets are the concept's terms in the target locale, allowed
	// ones first; empty when no target locale was asked for.
	Targets []TermUse `json:"targets,omitempty"`
}

// StyleRule is one explicit rule of the effective style guide.
type StyleRule struct {
	ID        string   `json:"id"`
	Title     string   `json:"title,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
	Good      []string `json:"good,omitempty"`
	Bad       []string `json:"bad,omitempty"`
}

// StyleGuideRef names a guide that was merged into the effective style.
type StyleGuideRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	// Scope is how the guide was scoped: "tenant", "project", a locale,
	// or "<locale>/<namespace>".
	Scope string `json:"scope"`
}

// Style is the effective style for a project, locale and namespace.
//
// Omitted: disabled rules, which are an authoring concept — a rule a
// narrower guide switched off is simply not in Rules.
type Style struct {
	// Fields are the mechanical fields the guides set — formality, tone,
	// punctuation, numbers, dates — merged and spelled exactly as the
	// style guide schema spells them. They are passed through rather
	// than reshaped: an agent asked to follow a style guide should see
	// the guide's own words, and every unset leaf inherits, so what is
	// absent here is genuinely unconstrained.
	Fields json.RawMessage `json:"fields"`
	Rules  []StyleRule     `json:"rules"`
	// Sources are the guides merged, broadest first.
	Sources []StyleGuideRef `json:"sources"`
}

// Knowledge is the Knowledge context, as MCP reads it.
type Knowledge interface {
	// SearchTM looks text up in translation memory for a locale pair.
	SearchTM(ctx context.Context, q TMSearch) ([]TMMatch, error)
	// LookupTerms recognizes termbase terms in a text.
	LookupTerms(ctx context.Context, q TermSearch) ([]Term, error)
	// Style returns the effective style guide.
	Style(ctx context.Context, project uuid.UUID, locale, namespace string) (Style, error)
}

// TMSearch is a translation-memory lookup.
type TMSearch struct {
	Project      uuid.UUID
	Text         string
	Syntax       string
	SourceLocale string
	TargetLocale string
	MessageKey   string
	Namespace    string
	Limit        int
	MinScore     int
}

// TermSearch is a termbase recognition request.
type TermSearch struct {
	Project      uuid.UUID
	Text         string
	Locale       string
	TargetLocale string
}

// ── Quality ─────────────────────────────────────────────────────────

// CheckRun is the stored run a findings page came from.
type CheckRun struct {
	ID  string `json:"id"`
	Ref string `json:"ref"`
	// Trigger is what asked for the run (cli, pull_request, write,
	// capture, api).
	Trigger    string `json:"trigger"`
	Conclusion string `json:"conclusion,omitempty"`
	// PolicyVersion is the check policy the run graded itself against.
	PolicyVersion int `json:"policy_version"`
	// Layers are the layers the run computed, so a caller can tell
	// "clean" from "not looked at".
	Layers      []string `json:"layers,omitempty"`
	Errors      int      `json:"errors"`
	Warnings    int      `json:"warnings"`
	Waived      int      `json:"waived"`
	StartedAt   string   `json:"started_at"`
	CompletedAt string   `json:"completed_at,omitempty"`
}

// Finding is one stored finding.
//
// Omitted: the evidence object and the fix hint, which are free-form
// per code and can be large, the byte spans, the capture and region,
// and the source revision. An agent reads what is wrong and where; the
// Quality API serves the full record.
type Finding struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
	Layer       string `json:"layer"`
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Key         string `json:"key,omitempty"`
	Locale      string `json:"locale,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
	// Explanation is the prose for a person; Subject the thing the
	// finding is about (the argument, the term, the markup element).
	Explanation string `json:"explanation"`
	Subject     string `json:"subject,omitempty"`
	// Waiver is the waiver that accepted this finding, when it is
	// waived.
	Waiver string `json:"waiver,omitempty"`
}

// FindingsQuery narrows findings_list.
type FindingsQuery struct {
	Ref        string
	Layer      string
	Locale     string
	Severity   string
	MessageKey string
	WaivedOnly bool
	Limit      int
	After      string
}

// Quality is the Quality context, as MCP reads it.
type Quality interface {
	// Findings returns a page of the latest check run's findings,
	// ErrNotFound when the project has no stored run.
	Findings(
		ctx context.Context, project uuid.UUID, q FindingsQuery,
	) (run CheckRun, findings []Finding, next string, err error)
}

// ── Release and delivery ────────────────────────────────────────────

// Served is what an environment serves: the release, and everything
// locale resolution reads from its manifest.
type Served struct {
	ReleaseID    string
	Version      int
	Digest       string
	CreatedAt    string
	Environment  string
	SourceLocale string
	Locales      []string
	Fallback     map[string][]string
	// Artifacts maps locale → namespace → the artifact's SHA-256.
	Artifacts map[string]map[string]string
}

// Delivery is the Release context, as MCP reads it.
type Delivery interface {
	// Served returns what environment currently serves, or the release
	// named by release when it is not uuid.Nil. ErrNotFound covers an
	// unknown project, environment or release, and an environment that
	// has never published.
	Served(ctx context.Context, project uuid.UUID, environment string, release uuid.UUID) (Served, error)
	// ArtifactHasMessage reports whether one artifact of a release
	// carries a message key.
	ArtifactHasMessage(ctx context.Context, project uuid.UUID, release uuid.UUID, digest, key string) (bool, error)
}

// Sources are the application ports the read tools call. A nil port
// leaves its tools unregistered, so a deployment that does not run a
// context does not advertise tools that cannot work.
type Sources struct {
	Catalog      Catalog
	Translations Translations
	Usages       UsageReader
	Knowledge    Knowledge
	Quality      Quality
	Delivery     Delivery
}
