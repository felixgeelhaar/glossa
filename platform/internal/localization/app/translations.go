package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
	"github.com/felixgeelhaar/glossa/platform/internal/localization/domain"

	"github.com/google/uuid"
)

// TranslationView is a translation with the message's current source
// revision, from which Outdated is derived.
type TranslationView struct {
	domain.Translation
	CurrentSourceRevision int
}

// Outdated reports whether the source moved on since the text was made.
func (v TranslationView) Outdated() bool { return v.Translation.Outdated(v.CurrentSourceRevision) }

func view(r TranslationRow) TranslationView {
	return TranslationView{Translation: r.Translation, CurrentSourceRevision: r.CurrentSourceRevision}
}

// TranslationInput is new text for a translation.
type TranslationInput struct {
	Text string
	// Syntax "" means the project's default.
	Syntax string
	// State is the requested review state; nil lets the project's
	// policy decide.
	State *string
	// Origin "" means human.
	Origin       string
	OriginDetail json.RawMessage
	// SourceRevision is the source revision the text was made against;
	// nil means the current one.
	SourceRevision *int
}

// WriteStatus is what a translation write did.
type WriteStatus string

// Write statuses.
const (
	WriteCreated   WriteStatus = "created"
	WriteRevised   WriteStatus = "revised"
	WriteReviewed  WriteStatus = "reviewed"
	WriteUnchanged WriteStatus = "unchanged"
)

// GetTranslation returns a message's translation in a locale.
func (s *Service) GetTranslation(ctx context.Context, project uuid.UUID, key, locale string) (TranslationView, error) {
	if err := authz.Require(ctx, authz.TranslationsRead); err != nil {
		return TranslationView{}, err
	}
	tag, err := localeFromPath(locale)
	if err != nil {
		return TranslationView{}, err
	}
	msg, err := s.catalog.Message(ctx, project, key)
	if err != nil {
		return TranslationView{}, err
	}
	var v TranslationView
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		row, err := st.Translation(ctx, msg.ID, tag)
		v = view(row)
		return err
	})
	return v, err
}

// ListTranslations lists a message's translations by locale.
func (s *Service) ListTranslations(ctx context.Context, project uuid.UUID, key string, page pagination.Page) ([]TranslationView, *string, error) {
	if err := authz.Require(ctx, authz.TranslationsRead); err != nil {
		return nil, nil, err
	}
	msg, err := s.catalog.Message(ctx, project, key)
	if err != nil {
		return nil, nil, err
	}
	var rows []TranslationView
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		stored, err := st.TranslationsOfMessage(ctx, msg.ID, page.After, page.Limit())
		for _, r := range stored {
			rows = append(rows, view(r))
		}
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	items, next := pagination.Trim(rows, page, func(v TranslationView) string { return v.Locale.String() })
	return items, next, nil
}

// writeCmd is a prepared, checked translation write.
type writeCmd struct {
	project   ProjectInfo
	msg       SourceMessage
	locale    bcp47.Tag
	write     domain.Write
	canReview bool
	// ifMatch applies the create-or-replace rule when enforce is set.
	ifMatch *int
	enforce bool
}

// prepare validates a write outside any transaction: it reads the
// source through Catalog's port, parses the text for the target locale
// and runs structural QA against the source revision it was made from.
func (s *Service) prepare(ctx context.Context, p ProjectInfo, msg SourceMessage, locale bcp47.Tag, in TranslationInput, defaultOrigin domain.Origin, by string) (writeCmd, error) {
	if locale == p.SourceLocale {
		return writeCmd{}, domain.ErrSourceLocale
	}
	sourceRev := msg.Revision
	if in.SourceRevision != nil {
		sourceRev = *in.SourceRevision
	}
	if sourceRev < 1 || sourceRev > msg.Revision {
		return writeCmd{}, domain.ErrInvalidSourceRev
	}
	source := msg.Content
	if sourceRev != msg.Revision {
		var err error
		if source, err = s.catalog.SourceAt(ctx, p.ID, msg.ID, sourceRev); err != nil {
			return writeCmd{}, err
		}
	}
	syntax, err := mfcontent.ParseSyntax(in.Syntax, p.DefaultSyntax)
	if err != nil {
		return writeCmd{}, err
	}
	content, err := mfcontent.Parse(syntax, in.Text, locale)
	if err != nil {
		return writeCmd{}, err
	}
	origin, err := domain.ParseOrigin(in.Origin, defaultOrigin)
	if err != nil {
		return writeCmd{}, err
	}
	prov, err := domain.NewProvenance(origin, in.OriginDetail, by)
	if err != nil {
		return writeCmd{}, err
	}
	w := domain.Write{
		Content: content, Provenance: prov, SourceRevision: sourceRev,
		QA: domain.CheckStructure(source, content, locale, msg.MaxLength),
	}
	if in.State != nil {
		st, err := domain.ParseReviewState(*in.State)
		if err != nil {
			return writeCmd{}, err
		}
		w.State = &st
	}
	if err := w.QA.Gate(); err != nil {
		return writeCmd{}, err
	}
	return writeCmd{
		project: p, msg: msg, locale: locale, write: w,
		canReview: allowedFor(ctx, authz.TranslationsReview, locale),
	}, nil
}

// PutTranslation creates or revises a message's translation in a
// locale. ifMatch must be the translation's ETag when it exists and
// absent when it doesn't. Error-severity structural findings reject the
// write (*domain.QAError); warnings are stored and returned.
func (s *Service) PutTranslation(ctx context.Context, project uuid.UUID, key, locale string, in TranslationInput, ifMatch *int) (TranslationView, WriteStatus, error) {
	tag, err := bcp47.Parse(locale)
	if err != nil {
		return TranslationView{}, "", err
	}
	by, err := actorFor(ctx, authz.TranslationsWrite, tag)
	if err != nil {
		return TranslationView{}, "", err
	}
	p, err := s.catalog.Project(ctx, project)
	if err != nil {
		return TranslationView{}, "", err
	}
	msg, err := s.catalog.Message(ctx, project, key)
	if err != nil {
		return TranslationView{}, "", err
	}
	cmd, err := s.prepare(ctx, p, msg, tag, in, domain.OriginHuman, by)
	if err != nil {
		return TranslationView{}, "", err
	}
	cmd.ifMatch, cmd.enforce = ifMatch, true
	var (
		v      TranslationView
		status WriteStatus
	)
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		if err := s.requireLocale(ctx, st, p, tag); err != nil {
			return err
		}
		t, stat, err := s.write(ctx, st, cmd)
		if err != nil {
			return err
		}
		v, status = TranslationView{Translation: t, CurrentSourceRevision: msg.Revision}, stat
		return nil
	})
	return v, status, err
}

// requireLocale checks that the project translates into locale.
func (s *Service) requireLocale(ctx context.Context, st Store, p ProjectInfo, locale bcp47.Tag) error {
	_, err := st.Locale(ctx, p.ID, locale)
	if errors.Is(err, ErrNotFound) {
		return ErrLocaleNotFound
	}
	return err
}

// write applies a prepared write in st: it brings the message
// projection up to the source it just read, then creates or revises the
// translation, appends the revision and publishes the event.
func (s *Service) write(ctx context.Context, st Store, cmd writeCmd) (domain.Translation, WriteStatus, error) {
	if err := s.applyMessageState(ctx, st, stateOf(cmd.msg), cmd.locale); err != nil {
		return domain.Translation{}, "", err
	}
	policy := domain.WritePolicy{ReviewRequired: cmd.project.ReviewRequired, Flow: s.flow}
	t, found, err := st.LockTranslation(ctx, cmd.msg.ID, cmd.locale)
	if err != nil {
		return domain.Translation{}, "", err
	}
	if cmd.enforce {
		version := 0
		if found {
			version = t.Revision
		}
		if err := checkIfMatch(version, cmd.ifMatch); err != nil {
			return domain.Translation{}, "", err
		}
	}
	if !found {
		t, rev, err := domain.NewTranslation(cmd.project.ID, cmd.msg.ID, cmd.locale, cmd.write, policy, cmd.canReview, s.now())
		if err != nil {
			return domain.Translation{}, "", err
		}
		if err := st.InsertTranslation(ctx, t); err != nil {
			return domain.Translation{}, "", err
		}
		return t, WriteCreated, s.record(ctx, st, t, rev)
	}
	expected := t.Revision
	rev, changed, err := t.Revise(cmd.write, policy, cmd.canReview, s.now())
	if err != nil || !changed {
		return t, WriteUnchanged, err
	}
	if err := st.UpdateTranslation(ctx, t, expected); err != nil {
		return domain.Translation{}, "", err
	}
	status := WriteRevised
	if rev.Kind == domain.KindReview {
		status = WriteReviewed
	}
	return t, status, s.record(ctx, st, t, rev)
}

// record appends a revision and publishes its event.
func (s *Service) record(ctx context.Context, st Store, t domain.Translation, rev domain.Revision) error {
	if err := st.AppendRevision(ctx, rev); err != nil {
		return err
	}
	typ := domain.EventTranslationRevised
	if rev.Kind == domain.KindReview {
		typ = domain.EventTranslationReviewed
	}
	return st.Publish(ctx, outbox.Event{
		Type: typ, AggregateType: domain.AggregateTranslation, AggregateID: t.ID.String(),
		Payload: domain.TranslationEventOf(t, rev.Provenance.By),
	})
}

// ReviewTranslation moves a translation to state as a review decision
// (approve, reject, send back to draft or review) if it is still at
// ifMatch. Approving and rejecting need translations.review for the
// locale.
func (s *Service) ReviewTranslation(ctx context.Context, project uuid.UUID, key, locale, state string, ifMatch int) (TranslationView, error) {
	tag, err := localeFromPath(locale)
	if err != nil {
		return TranslationView{}, err
	}
	by, err := actorFor(ctx, authz.TranslationsWrite, tag)
	if err != nil {
		return TranslationView{}, err
	}
	to, err := domain.ParseReviewState(state)
	if err != nil {
		return TranslationView{}, err
	}
	msg, err := s.catalog.Message(ctx, project, key)
	if err != nil {
		return TranslationView{}, err
	}
	canReview := allowedFor(ctx, authz.TranslationsReview, tag)
	var v TranslationView
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		t, found, err := st.LockTranslation(ctx, msg.ID, tag)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if t.Revision != ifMatch {
			return ErrPreconditionFailed
		}
		rev, err := t.Review(to, by, s.flow, canReview, s.now())
		if err != nil {
			return err
		}
		if err := st.UpdateTranslation(ctx, t, ifMatch); err != nil {
			return err
		}
		v = TranslationView{Translation: t, CurrentSourceRevision: msg.Revision}
		return s.record(ctx, st, t, rev)
	})
	return v, err
}

// TranslationRevisions lists a translation's log, newest first.
func (s *Service) TranslationRevisions(ctx context.Context, project uuid.UUID, key, locale string, page pagination.Page) ([]domain.Revision, *string, error) {
	if err := authz.Require(ctx, authz.TranslationsRead); err != nil {
		return nil, nil, err
	}
	tag, err := localeFromPath(locale)
	if err != nil {
		return nil, nil, err
	}
	before, err := beforeRevision(page.After)
	if err != nil {
		return nil, nil, err
	}
	msg, err := s.catalog.Message(ctx, project, key)
	if err != nil {
		return nil, nil, err
	}
	var rows []domain.Revision
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		t, err := st.Translation(ctx, msg.ID, tag)
		if err != nil {
			return err
		}
		rows, err = st.Revisions(ctx, t.ID, before, page.Limit())
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	items, next := pagination.Trim(rows, page, func(r domain.Revision) string { return strconv.Itoa(r.Number) })
	return items, next, nil
}
