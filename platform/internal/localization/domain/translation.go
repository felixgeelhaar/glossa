package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
)

// ReviewState is where a translation is in review.
type ReviewState string

// Review states.
const (
	StateDraft       ReviewState = "draft"
	StateNeedsReview ReviewState = "needs_review"
	StateApproved    ReviewState = "approved"
	StateRejected    ReviewState = "rejected"
)

// ParseReviewState validates s.
func ParseReviewState(s string) (ReviewState, error) {
	switch ReviewState(s) {
	case StateDraft, StateNeedsReview, StateApproved, StateRejected:
		return ReviewState(s), nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidReviewState, s)
}

// ReviewFlow is the review process as data: which state changes are
// allowed and which target states are a reviewer's decision. It is the
// seam where the Phase-4 workflow engine (a statekit statechart per
// project, RFC 0002 §4) takes over; nothing else in this package
// hard-codes review rules.
type ReviewFlow struct {
	Transitions map[ReviewState][]ReviewState
	// ReviewerOnly lists states only someone with the review permission
	// may put a translation in.
	ReviewerOnly []ReviewState
}

// DefaultReviewFlow lets any state move to any other; approving and
// rejecting are reviewer decisions.
func DefaultReviewFlow() ReviewFlow {
	all := []ReviewState{StateDraft, StateNeedsReview, StateApproved, StateRejected}
	t := map[ReviewState][]ReviewState{}
	for _, from := range all {
		t[from] = slices.DeleteFunc(slices.Clone(all), func(to ReviewState) bool { return to == from })
	}
	return ReviewFlow{Transitions: t, ReviewerOnly: []ReviewState{StateApproved, StateRejected}}
}

// Allows reports whether from → to is a permitted change.
func (f ReviewFlow) Allows(from, to ReviewState) bool {
	return slices.Contains(f.Transitions[from], to)
}

// NeedsReviewer reports whether only a reviewer may move to s.
func (f ReviewFlow) NeedsReviewer(s ReviewState) bool { return slices.Contains(f.ReviewerOnly, s) }

// WritePolicy decides the review state a new translation text gets.
type WritePolicy struct {
	// ReviewRequired is the project setting: new text waits for review
	// unless a reviewer approves it as they write it.
	ReviewRequired bool
	Flow           ReviewFlow
}

// StateOnWrite returns the state for new text. requested is what the
// writer asked for (nil: the policy's default); canReview says whether
// they hold the review permission for the locale. New text never keeps
// an earlier approval: it is re-decided here.
func (p WritePolicy) StateOnWrite(requested *ReviewState, canReview bool) (ReviewState, error) {
	if requested == nil {
		if p.ReviewRequired {
			return StateNeedsReview, nil
		}
		return StateApproved, nil
	}
	switch s := *requested; {
	case s == StateRejected:
		return "", ErrWriteCannotReject
	case p.Flow.NeedsReviewer(s) && p.ReviewRequired && !canReview:
		return "", ErrReviewForbidden
	default:
		return s, nil
	}
}

// Origin says how a translation revision came to be (intent §22).
type Origin string

// Origins.
const (
	OriginHuman              Origin = "human"
	OriginAI                 Origin = "ai"
	OriginTranslationMemory  Origin = "translation_memory"
	OriginMachineTranslation Origin = "machine_translation"
	OriginImport             Origin = "import"
	OriginAdaptation         Origin = "adaptation"
)

// ParseOrigin validates s; "" means def.
func ParseOrigin(s string, def Origin) (Origin, error) {
	switch Origin(s) {
	case "":
		return def, nil
	case OriginHuman, OriginAI, OriginTranslationMemory, OriginMachineTranslation, OriginImport, OriginAdaptation:
		return Origin(s), nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidOrigin, s)
}

// MaxOriginDetail bounds a revision's origin detail.
const MaxOriginDetail = 16 << 10

// Provenance records why a revision exists: how it was made (Origin),
// the specifics (model and prompt version, TM match, import job, the
// locale it was adapted from …) and who made it.
type Provenance struct {
	Origin Origin
	// Detail is a JSON object; "{}" when there is nothing to add.
	Detail json.RawMessage
	// By is the acting principal, "person:<id>" or "token:<id>".
	By string
}

// NewProvenance validates detail (nil or a JSON object).
func NewProvenance(origin Origin, detail json.RawMessage, by string) (Provenance, error) {
	if len(bytes.TrimSpace(detail)) == 0 {
		detail = json.RawMessage(`{}`)
	}
	var obj map[string]any
	if len(detail) > MaxOriginDetail || json.Unmarshal(detail, &obj) != nil || obj == nil {
		return Provenance{}, ErrInvalidOriginInfo
	}
	return Provenance{Origin: origin, Detail: detail, By: by}, nil
}

// RevisionKind distinguishes new text from a review decision in the log.
type RevisionKind string

// Revision kinds.
const (
	KindContent RevisionKind = "content"
	KindReview  RevisionKind = "review"
)

// Revision is one entry of a translation's append-only log: a full
// snapshot (text, state, source revision) plus who did it and how. The
// translation's current state is always its latest revision.
type Revision struct {
	TranslationID  TranslationID
	Number         int
	Kind           RevisionKind
	Content        mfcontent.Content
	State          ReviewState
	Provenance     Provenance
	SourceRevision int
	// Findings are the structural QA warnings the text had when written.
	Findings  []mf.Finding
	CreatedAt time.Time
}

// Translation is one message's current text in one locale. Whether it
// is outdated is not stored: it is outdated exactly when the source
// revision it was made against is older than the message's current one.
type Translation struct {
	ID        TranslationID
	ProjectID uuid.UUID
	MessageID uuid.UUID
	Locale    bcp47.Tag
	Content   mfcontent.Content
	State     ReviewState
	// Origin and By are the provenance of the current text.
	Origin Origin
	By     string
	// SourceRevision is the source revision the text was made against.
	SourceRevision int
	Warnings       []mf.Finding
	// Revision is the number of the latest log entry; Version is the
	// ETag and equals Revision (every change appends to the log).
	Revision  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Outdated reports whether the source changed since this text was made.
func (t Translation) Outdated(currentSourceRevision int) bool {
	return t.SourceRevision < currentSourceRevision
}

// Write is new text for a translation.
type Write struct {
	Content        mfcontent.Content
	Provenance     Provenance
	SourceRevision int
	// State is the requested review state; nil means the policy default.
	State *ReviewState
	// QA is the structural check of Content against the source.
	QA QAResult
}

// NewTranslation creates the first revision of message's text in locale.
func NewTranslation(project, message uuid.UUID, locale bcp47.Tag, w Write, policy WritePolicy, canReview bool, now time.Time) (Translation, Revision, error) {
	if err := w.QA.Gate(); err != nil {
		return Translation{}, Revision{}, err
	}
	state, err := policy.StateOnWrite(w.State, canReview)
	if err != nil {
		return Translation{}, Revision{}, err
	}
	t := Translation{
		ID: NewTranslationID(), ProjectID: project, MessageID: message, Locale: locale, CreatedAt: now,
	}
	return t, t.apply(KindContent, w.Content, state, w.Provenance, w.SourceRevision, w.QA.Warnings, now), nil
}

// Revise records new text. Text with the same model, made against the
// same source revision, is no new revision unless it asks for a
// different review state — then it is a review decision. Re-submitting
// unchanged text against a newer source revision is a content revision:
// it confirms the text still fits and clears "outdated".
func (t *Translation) Revise(w Write, policy WritePolicy, canReview bool, now time.Time) (Revision, bool, error) {
	if err := w.QA.Gate(); err != nil {
		return Revision{}, false, err
	}
	if t.Content.SameModel(w.Content) && t.SourceRevision == w.SourceRevision {
		if w.State == nil || *w.State == t.State {
			return Revision{}, false, nil
		}
		rev, err := t.Review(*w.State, w.Provenance.By, policy.Flow, canReview, now)
		return rev, err == nil, err
	}
	state, err := policy.StateOnWrite(w.State, canReview)
	if err != nil {
		return Revision{}, false, err
	}
	return t.apply(KindContent, w.Content, state, w.Provenance, w.SourceRevision, w.QA.Warnings, now), true, nil
}

// Review moves the translation to state to, as a review decision by by.
func (t *Translation) Review(to ReviewState, by string, flow ReviewFlow, canReview bool, now time.Time) (Revision, error) {
	if !flow.Allows(t.State, to) {
		return Revision{}, fmt.Errorf("%w: %s → %s", ErrTransition, t.State, to)
	}
	if flow.NeedsReviewer(to) && !canReview {
		return Revision{}, ErrReviewForbidden
	}
	// The text's provenance stays; the log entry names the reviewer.
	prov := Provenance{Origin: t.Origin, Detail: json.RawMessage(`{}`), By: by}
	rev := t.apply(KindReview, t.Content, to, prov, t.SourceRevision, t.Warnings, now)
	return rev, nil
}

func (t *Translation) apply(kind RevisionKind, c mfcontent.Content, state ReviewState, prov Provenance, sourceRev int, warnings []mf.Finding, now time.Time) Revision {
	t.Revision++
	t.Content, t.State, t.SourceRevision, t.Warnings, t.UpdatedAt = c, state, sourceRev, warnings, now
	if kind == KindContent {
		t.Origin, t.By = prov.Origin, prov.By
	}
	return Revision{
		TranslationID: t.ID, Number: t.Revision, Kind: kind, Content: c, State: state, Provenance: prov,
		SourceRevision: sourceRev, Findings: warnings, CreatedAt: now,
	}
}
