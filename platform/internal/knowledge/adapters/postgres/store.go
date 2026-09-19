// Package postgres implements Knowledge's persistence port on the
// kernel's unit of work, with sqlc queries over the knowledge_* tables.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/knowledge/adapters/postgres/knowledgesql"
	"github.com/felixgeelhaar/glossa/platform/internal/knowledge/app"
	"github.com/felixgeelhaar/glossa/platform/internal/knowledge/domain"
)

// Transactor implements app.Transactor.
type Transactor struct{ uow *db.UnitOfWork }

// NewTransactor returns a Transactor on uow.
func NewTransactor(uow *db.UnitOfWork) *Transactor { return &Transactor{uow: uow} }

// InTenant implements app.Transactor.
func (t *Transactor) InTenant(ctx context.Context, fn func(context.Context, app.Store) error) error {
	return t.uow.InTenantTx(ctx, func(ctx context.Context, tx *db.TenantTx) error {
		return fn(ctx, &store{tx: tx, q: knowledgesql.New(tx)})
	})
}

type store struct {
	tx *db.TenantTx
	q  *knowledgesql.Queries
}

var _ app.Store = (*store)(nil)

func storeError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "knowledge_style_guides_scope" {
		return app.ErrStyleGuideExists
	}
	return err
}

func int32Of(n int) int32 { return int32(n) } //nolint:gosec // versions, revisions and page sizes stay small

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	id := n.UUID
	return &id
}

func nullText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func nullTag(t *bcp47.Tag) pgtype.Text {
	if t == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: t.String(), Valid: true}
}

func parseTag(s string) (bcp47.Tag, error) {
	t, err := bcp47.Parse(s)
	if err != nil {
		return bcp47.Tag{}, fmt.Errorf("knowledge: stored locale %q: %w", s, err)
	}
	return t, nil
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
}

// ── translation memory ──────────────────────────────────────────────

func (s *store) EnsureDerivation(ctx context.Context, translation, project uuid.UUID, at time.Time) error {
	return storeError(s.q.EnsureDerivation(ctx, knowledgesql.EnsureDerivationParams{
		TranslationID: translation, ProjectID: project, UpdatedAt: at,
	}))
}

func (s *store) LockDerivation(ctx context.Context, translation uuid.UUID) (int, error) {
	n, err := s.q.LockDerivation(ctx, translation)
	return int(n), storeError(err)
}

func (s *store) SetDerivation(ctx context.Context, translation uuid.UUID, revision int, at time.Time) error {
	return storeError(s.q.SetDerivation(ctx, knowledgesql.SetDerivationParams{
		TranslationID: translation, Revision: int32Of(revision), UpdatedAt: at,
	}))
}

func unit(r knowledgesql.KnowledgeTmUnit) (domain.TMUnit, error) {
	source, err := parseTag(r.SourceLocale)
	if err != nil {
		return domain.TMUnit{}, err
	}
	target, err := parseTag(r.TargetLocale)
	if err != nil {
		return domain.TMUnit{}, err
	}
	var model mf.Message
	if err := json.Unmarshal(r.TargetModel, &model); err != nil {
		return domain.TMUnit{}, fmt.Errorf("knowledge: stored target of unit %s: %w", r.ID, err)
	}
	var vars []string
	if err := json.Unmarshal(r.SourceVars, &vars); err != nil {
		return domain.TMUnit{}, fmt.Errorf("knowledge: stored variables of unit %s: %w", r.ID, err)
	}
	u := domain.TMUnit{
		ID: r.ID, ProjectID: uuidPtr(r.ProjectID), Origin: domain.UnitOrigin(r.Origin),
		TranslationID: uuidPtr(r.TranslationID), TranslationRevision: int(r.TranslationRevision.Int32),
		MessageID: uuidPtr(r.MessageID), MessageKey: r.MessageKey, Namespace: r.Namespace,
		SourceLocale: source, TargetLocale: target, SourceMF2: r.SourceMf2, TargetMF2: r.TargetMf2, Target: model,
		SourceNorm: domain.Normalized{Text: r.SourceNormalized, Hash: r.SourceHash, Signature: r.Signature, Vars: vars},
		TargetNorm: domain.Normalized{Text: r.TargetNormalized},
		HitCount:   int(r.HitCount), LastHitAt: timePtr(r.LastHitAt),
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
		RetiredAt: timePtr(r.RetiredAt), RetiredReason: domain.RetireReason(r.RetiredReason.String),
		RetiredBy: r.RetiredBy.String,
	}
	return u, nil
}

func units(rows []knowledgesql.KnowledgeTmUnit) ([]domain.TMUnit, error) {
	out := make([]domain.TMUnit, 0, len(rows))
	for _, r := range rows {
		u, err := unit(r)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

func (s *store) LockActiveUnit(ctx context.Context, translation uuid.UUID) (*domain.TMUnit, error) {
	r, err := s.q.LockActiveUnitOfTranslation(ctx, uuid.NullUUID{UUID: translation, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, storeError(err)
	}
	u, err := unit(r)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *store) InsertUnit(ctx context.Context, u domain.TMUnit) error {
	model, err := json.Marshal(u.Target)
	if err != nil {
		return fmt.Errorf("knowledge: encode unit %s: %w", u.ID, err)
	}
	vars := u.SourceNorm.Vars
	if vars == nil {
		vars = []string{}
	}
	varsJSON, err := json.Marshal(vars)
	if err != nil {
		return err
	}
	rev := pgtype.Int4{}
	if u.TranslationRevision > 0 {
		rev = pgtype.Int4{Int32: int32Of(u.TranslationRevision), Valid: true}
	}
	return storeError(s.q.InsertTMUnit(ctx, knowledgesql.InsertTMUnitParams{
		ID: u.ID, ProjectID: nullUUID(u.ProjectID), Origin: string(u.Origin), TranslationID: nullUUID(u.TranslationID),
		TranslationRevision: rev, MessageID: nullUUID(u.MessageID), MessageKey: u.MessageKey, Namespace: u.Namespace,
		SourceLocale: u.SourceLocale.String(), TargetLocale: u.TargetLocale.String(), SourceMf2: u.SourceMF2,
		TargetMf2: u.TargetMF2, TargetModel: model, SourceNormalized: u.SourceNorm.Text,
		TargetNormalized: u.TargetNorm.Text, SourceHash: u.SourceNorm.Hash, Signature: u.SourceNorm.Signature,
		SourceVars: varsJSON, CreatedBy: u.CreatedBy, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}))
}

func (s *store) TouchUnit(ctx context.Context, id uuid.UUID, revision int, at time.Time) error {
	return storeError(s.q.TouchTMUnit(ctx, knowledgesql.TouchTMUnitParams{
		ID: id, TranslationRevision: pgtype.Int4{Int32: int32Of(revision), Valid: true}, UpdatedAt: at,
	}))
}

func (s *store) RetireUnit(ctx context.Context, u domain.TMUnit) error {
	if u.RetiredAt == nil {
		return fmt.Errorf("knowledge: unit %s is not retired", u.ID)
	}
	n, err := s.q.RetireTMUnit(ctx, knowledgesql.RetireTMUnitParams{
		ID: u.ID, RetiredAt: pgtype.Timestamptz{Time: *u.RetiredAt, Valid: true},
		RetiredReason: nullText(string(u.RetiredReason)), RetiredBy: nullText(u.RetiredBy),
	})
	if err != nil {
		return storeError(err)
	}
	if n == 0 {
		return app.ErrNotFound
	}
	return nil
}

func (s *store) Unit(ctx context.Context, id uuid.UUID) (domain.TMUnit, error) {
	r, err := s.q.GetTMUnit(ctx, id)
	if err != nil {
		return domain.TMUnit{}, storeError(err)
	}
	return unit(r)
}

func (s *store) LockUnit(ctx context.Context, id uuid.UUID) (domain.TMUnit, error) {
	r, err := s.q.LockTMUnit(ctx, id)
	if err != nil {
		return domain.TMUnit{}, storeError(err)
	}
	return unit(r)
}

func (s *store) Units(ctx context.Context, f app.UnitFilter, after uuid.UUID, limit int) ([]domain.TMUnit, error) {
	rows, err := s.q.ListTMUnits(ctx, knowledgesql.ListTMUnitsParams{
		After: after, SourceLocale: nullTag(f.SourceLocale), TargetLocale: nullTag(f.TargetLocale),
		ProjectID: nullUUID(f.ProjectID), TranslationID: nullUUID(f.TranslationID), State: f.State,
		MaxRows: int32Of(limit),
	})
	if err != nil {
		return nil, storeError(err)
	}
	return units(rows)
}

func (s *store) ExactMatches(ctx context.Context, sc app.MatchScope, source, target bcp47.Tag, n domain.Normalized, limit int) ([]domain.TMUnit, error) {
	rows, err := s.q.ExactTMMatches(ctx, knowledgesql.ExactTMMatchesParams{
		SourceLocale: source.String(), TargetLocale: target.String(), SourceHash: n.Hash, Signature: n.Signature,
		AllProjects: sc.AllProjects, ProjectID: nullUUID(sc.ProjectID), MaxRows: int32Of(limit),
	})
	if err != nil {
		return nil, storeError(err)
	}
	return units(rows)
}

func (s *store) FuzzyMatches(ctx context.Context, sc app.MatchScope, source, target bcp47.Tag, text string, minSimilarity float64, limit int) ([]app.ScoredUnit, error) {
	if err := s.q.SetSimilarityThreshold(ctx, strconv.FormatFloat(minSimilarity, 'f', 4, 64)); err != nil {
		return nil, storeError(err)
	}
	rows, err := s.q.FuzzyTMMatches(ctx, knowledgesql.FuzzyTMMatchesParams{
		Query: text, SourceLocale: source.String(), TargetLocale: target.String(), AllProjects: sc.AllProjects,
		ProjectID: nullUUID(sc.ProjectID), MaxRows: int32Of(limit),
	})
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]app.ScoredUnit, 0, len(rows))
	for _, r := range rows {
		u, err := unit(r.KnowledgeTmUnit)
		if err != nil {
			return nil, err
		}
		out = append(out, app.ScoredUnit{TMUnit: u, Similarity: r.Similarity})
	}
	return out, nil
}

// likePattern matches q anywhere, with LIKE's wildcards escaped.
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

func (s *store) Concordance(ctx context.Context, f app.ConcordanceFilter) ([]app.ScoredUnit, error) {
	var (
		rows []app.ScoredUnit
		add  = func(r knowledgesql.KnowledgeTmUnit, sim float64) error {
			u, err := unit(r)
			rows = append(rows, app.ScoredUnit{TMUnit: u, Similarity: sim})
			return err
		}
	)
	if f.Side == domain.SideTarget {
		found, err := s.q.ConcordanceTarget(ctx, knowledgesql.ConcordanceTargetParams{
			Query: f.Query, SourceLocale: nullTag(f.SourceLocale), TargetLocale: nullTag(f.TargetLocale),
			AllProjects: f.AllProjects, ProjectID: nullUUID(f.ProjectID), Pattern: likePattern(f.Query),
			MaxRows: int32Of(f.Limit),
		})
		if err != nil {
			return nil, storeError(err)
		}
		for _, r := range found {
			if err := add(r.KnowledgeTmUnit, r.Similarity); err != nil {
				return nil, err
			}
		}
		return rows, nil
	}
	found, err := s.q.ConcordanceSource(ctx, knowledgesql.ConcordanceSourceParams{
		Query: f.Query, SourceLocale: nullTag(f.SourceLocale), TargetLocale: nullTag(f.TargetLocale),
		AllProjects: f.AllProjects, ProjectID: nullUUID(f.ProjectID), Pattern: likePattern(f.Query),
		MaxRows: int32Of(f.Limit),
	})
	if err != nil {
		return nil, storeError(err)
	}
	for _, r := range found {
		if err := add(r.KnowledgeTmUnit, r.Similarity); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (s *store) CountHits(ctx context.Context, ids []uuid.UUID, at time.Time) error {
	return storeError(s.q.CountTMHits(ctx, knowledgesql.CountTMHitsParams{Ids: ids, HitAt: pgtype.Timestamptz{Time: at, Valid: true}}))
}

// ── termbase ────────────────────────────────────────────────────────

func concept(r knowledgesql.KnowledgeConcept, terms []knowledgesql.KnowledgeTerm) (domain.Concept, error) {
	c := domain.Concept{
		ID: r.ID, ProjectID: uuidPtr(r.ProjectID), Definition: r.Definition, Domain: r.Domain, Note: r.Note,
		ProductRef: r.ProductRef, Version: int(r.Version), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
		UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt.UTC(),
	}
	for _, t := range terms {
		loc, err := parseTag(t.Locale)
		if err != nil {
			return domain.Concept{}, err
		}
		c.Terms = append(c.Terms, domain.Term{
			ID: t.ID, Locale: loc, Text: t.Text, Status: domain.TermStatus(t.Status),
			PartOfSpeech: domain.PartOfSpeech(t.PartOfSpeech), CaseSensitive: t.CaseSensitive, Note: t.Note,
		})
	}
	return c, nil
}

// withTerms loads the terms of rows and assembles the concepts.
func (s *store) withTerms(ctx context.Context, rows []knowledgesql.KnowledgeConcept) ([]domain.Concept, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	terms, err := s.q.TermsOfConcepts(ctx, ids)
	if err != nil {
		return nil, storeError(err)
	}
	byConcept := map[uuid.UUID][]knowledgesql.KnowledgeTerm{}
	for _, t := range terms {
		byConcept[t.ConceptID] = append(byConcept[t.ConceptID], t)
	}
	out := make([]domain.Concept, 0, len(rows))
	for _, r := range rows {
		c, err := concept(r, byConcept[r.ID])
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *store) insertTerms(ctx context.Context, c domain.Concept) error {
	for i, t := range c.Terms {
		if err := s.q.InsertTerm(ctx, knowledgesql.InsertTermParams{
			ID: t.ID, ConceptID: c.ID, Position: int32Of(i), Locale: t.Locale.String(), Text: t.Text,
			Status: string(t.Status), PartOfSpeech: string(t.PartOfSpeech), CaseSensitive: t.CaseSensitive, Note: t.Note,
		}); err != nil {
			return storeError(err)
		}
	}
	return nil
}

func (s *store) InsertConcept(ctx context.Context, c domain.Concept) (bool, error) {
	n, err := s.q.InsertConcept(ctx, knowledgesql.InsertConceptParams{
		ID: c.ID, ProjectID: nullUUID(c.ProjectID), Definition: c.Definition, Domain: c.Domain, Note: c.Note,
		ProductRef: c.ProductRef, Version: int32Of(c.Version), CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt,
		UpdatedBy: c.UpdatedBy, UpdatedAt: c.UpdatedAt,
	})
	if err != nil || n == 0 {
		return false, storeError(err)
	}
	return true, s.insertTerms(ctx, c)
}

func (s *store) UpdateConcept(ctx context.Context, c domain.Concept, expected int) error {
	n, err := s.q.UpdateConcept(ctx, knowledgesql.UpdateConceptParams{
		ID: c.ID, Definition: c.Definition, Domain: c.Domain, Note: c.Note, ProductRef: c.ProductRef,
		Version: int32Of(c.Version), UpdatedBy: c.UpdatedBy, UpdatedAt: c.UpdatedAt, ExpectedVersion: int32Of(expected),
	})
	if err != nil {
		return storeError(err)
	}
	if n == 0 {
		return app.ErrStaleVersion
	}
	if err := s.q.DeleteTermsOfConcept(ctx, c.ID); err != nil {
		return storeError(err)
	}
	return s.insertTerms(ctx, c)
}

func (s *store) DeleteConcept(ctx context.Context, id uuid.UUID) error {
	return storeError(s.q.DeleteConcept(ctx, id))
}

func (s *store) Concept(ctx context.Context, id uuid.UUID) (domain.Concept, error) {
	r, err := s.q.GetConcept(ctx, id)
	if err != nil {
		return domain.Concept{}, storeError(err)
	}
	cs, err := s.withTerms(ctx, []knowledgesql.KnowledgeConcept{r})
	if err != nil {
		return domain.Concept{}, err
	}
	return cs[0], nil
}

func (s *store) LockConcept(ctx context.Context, id uuid.UUID) (domain.Concept, error) {
	r, err := s.q.LockConcept(ctx, id)
	if err != nil {
		return domain.Concept{}, storeError(err)
	}
	cs, err := s.withTerms(ctx, []knowledgesql.KnowledgeConcept{r})
	if err != nil {
		return domain.Concept{}, err
	}
	return cs[0], nil
}

func (s *store) Concepts(ctx context.Context, f app.ConceptFilter, after uuid.UUID, limit int) ([]domain.Concept, error) {
	p := knowledgesql.ListConceptsParams{
		After: after, ProjectID: nullUUID(f.ProjectID), Locale: nullTag(f.Locale), MaxRows: int32Of(limit),
	}
	if f.Domain != nil {
		p.Domain = pgtype.Text{String: *f.Domain, Valid: true}
	}
	if f.Query != nil {
		p.Pattern = pgtype.Text{String: likePattern(*f.Query), Valid: true}
	}
	rows, err := s.q.ListConcepts(ctx, p)
	if err != nil {
		return nil, storeError(err)
	}
	return s.withTerms(ctx, rows)
}

func (s *store) ConceptsWithTermsIn(ctx context.Context, project *uuid.UUID, locales []bcp47.Tag) ([]domain.Concept, error) {
	ls := make([]string, len(locales))
	for i, l := range locales {
		ls[i] = l.String()
	}
	rows, err := s.q.ConceptsWithTermsIn(ctx, knowledgesql.ConceptsWithTermsInParams{ProjectID: nullUUID(project), Locales: ls})
	if err != nil {
		return nil, storeError(err)
	}
	return s.withTerms(ctx, rows)
}

// conceptSnapshot is a concept as its history stores it.
type conceptSnapshot struct {
	ID         uuid.UUID      `json:"id"`
	ProjectID  *uuid.UUID     `json:"project_id,omitempty"`
	Definition string         `json:"definition"`
	Domain     string         `json:"domain"`
	Note       string         `json:"note"`
	ProductRef string         `json:"product_ref"`
	Terms      []termSnapshot `json:"terms"`
	Version    int            `json:"version"`
	CreatedBy  string         `json:"created_by"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedBy  string         `json:"updated_by"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type termSnapshot struct {
	ID            uuid.UUID `json:"id"`
	Locale        string    `json:"locale"`
	Text          string    `json:"text"`
	Status        string    `json:"status"`
	PartOfSpeech  string    `json:"part_of_speech,omitempty"`
	CaseSensitive bool      `json:"case_sensitive"`
	Note          string    `json:"note,omitempty"`
}

func (s *store) AppendConceptRevision(ctx context.Context, r app.ConceptRevision) error {
	c := r.Concept
	snap := conceptSnapshot{
		ID: c.ID, ProjectID: c.ProjectID, Definition: c.Definition, Domain: c.Domain, Note: c.Note,
		ProductRef: c.ProductRef, Terms: []termSnapshot{}, Version: c.Version, CreatedBy: c.CreatedBy,
		CreatedAt: c.CreatedAt, UpdatedBy: c.UpdatedBy, UpdatedAt: c.UpdatedAt,
	}
	for _, t := range c.Terms {
		snap.Terms = append(snap.Terms, termSnapshot{
			ID: t.ID, Locale: t.Locale.String(), Text: t.Text, Status: string(t.Status),
			PartOfSpeech: string(t.PartOfSpeech), CaseSensitive: t.CaseSensitive, Note: t.Note,
		})
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return storeError(s.q.InsertConceptRevision(ctx, knowledgesql.InsertConceptRevisionParams{
		ConceptID: c.ID, ProjectID: nullUUID(c.ProjectID), Version: int32Of(c.Version), Action: string(r.Action),
		Snapshot: b, Author: r.Author, CreatedAt: r.CreatedAt,
	}))
}

func (s *store) ConceptRevisions(ctx context.Context, id uuid.UUID, before, limit int) ([]app.ConceptRevision, error) {
	rows, err := s.q.ListConceptRevisions(ctx, knowledgesql.ListConceptRevisionsParams{
		ConceptID: id, Before: int32Of(before), MaxRows: int32Of(limit),
	})
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]app.ConceptRevision, 0, len(rows))
	for _, r := range rows {
		var snap conceptSnapshot
		if err := json.Unmarshal(r.Snapshot, &snap); err != nil {
			return nil, fmt.Errorf("knowledge: stored revision %d of concept %s: %w", r.Version, id, err)
		}
		c := domain.Concept{
			ID: snap.ID, ProjectID: snap.ProjectID, Definition: snap.Definition, Domain: snap.Domain, Note: snap.Note,
			ProductRef: snap.ProductRef, Version: snap.Version, CreatedBy: snap.CreatedBy, CreatedAt: snap.CreatedAt.UTC(),
			UpdatedBy: snap.UpdatedBy, UpdatedAt: snap.UpdatedAt.UTC(),
		}
		for _, t := range snap.Terms {
			loc, err := parseTag(t.Locale)
			if err != nil {
				return nil, err
			}
			c.Terms = append(c.Terms, domain.Term{
				ID: t.ID, Locale: loc, Text: t.Text, Status: domain.TermStatus(t.Status),
				PartOfSpeech: domain.PartOfSpeech(t.PartOfSpeech), CaseSensitive: t.CaseSensitive, Note: t.Note,
			})
		}
		out = append(out, app.ConceptRevision{Concept: c, Action: app.RevisionAction(r.Action), Author: r.Author, CreatedAt: r.CreatedAt.UTC()})
	}
	return out, nil
}

// ── style guides ────────────────────────────────────────────────────

func styleScope(project uuid.NullUUID, locale, namespace pgtype.Text) (domain.StyleScope, error) {
	sc := domain.StyleScope{ProjectID: uuidPtr(project), Namespace: namespace.String}
	if locale.Valid {
		t, err := parseTag(locale.String)
		if err != nil {
			return domain.StyleScope{}, err
		}
		sc.Locale = &t
	}
	return sc, nil
}

func styleContent(fields, rules json.RawMessage) (domain.StyleContent, error) {
	var c domain.StyleContent
	if err := json.Unmarshal(fields, &c.Fields); err != nil {
		return c, fmt.Errorf("knowledge: stored style fields: %w", err)
	}
	if err := json.Unmarshal(rules, &c.Rules); err != nil {
		return c, fmt.Errorf("knowledge: stored style rules: %w", err)
	}
	return c, nil
}

func styleGuide(r knowledgesql.KnowledgeStyleGuide) (domain.StyleGuide, error) {
	sc, err := styleScope(r.ProjectID, r.Locale, r.Namespace)
	if err != nil {
		return domain.StyleGuide{}, err
	}
	content, err := styleContent(r.Fields, r.Rules)
	if err != nil {
		return domain.StyleGuide{}, err
	}
	return domain.StyleGuide{
		ID: r.ID, Scope: sc, Name: r.Name, StyleContent: content, Version: int(r.Version),
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt.UTC(),
	}, nil
}

func styleGuides(rows []knowledgesql.KnowledgeStyleGuide) ([]domain.StyleGuide, error) {
	out := make([]domain.StyleGuide, 0, len(rows))
	for _, r := range rows {
		g, err := styleGuide(r)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func encodeStyle(c domain.StyleContent) (fields, rules []byte, err error) {
	if fields, err = json.Marshal(c.Fields); err != nil {
		return nil, nil, err
	}
	rs := c.Rules
	if rs == nil {
		rs = []domain.StyleRule{}
	}
	rules, err = json.Marshal(rs)
	return fields, rules, err
}

func (s *store) InsertStyleGuide(ctx context.Context, g domain.StyleGuide) (bool, error) {
	fields, rules, err := encodeStyle(g.StyleContent)
	if err != nil {
		return false, err
	}
	n, err := s.q.InsertStyleGuide(ctx, knowledgesql.InsertStyleGuideParams{
		ID: g.ID, ProjectID: nullUUID(g.Scope.ProjectID), Locale: nullTag(g.Scope.Locale),
		Namespace: nullText(g.Scope.Namespace), Name: g.Name, Fields: fields, Rules: rules, Version: int32Of(g.Version),
		CreatedBy: g.CreatedBy, CreatedAt: g.CreatedAt, UpdatedBy: g.UpdatedBy, UpdatedAt: g.UpdatedAt,
	})
	return n == 1, storeError(err)
}

func (s *store) UpdateStyleGuide(ctx context.Context, g domain.StyleGuide, expected int) error {
	fields, rules, err := encodeStyle(g.StyleContent)
	if err != nil {
		return err
	}
	n, err := s.q.UpdateStyleGuide(ctx, knowledgesql.UpdateStyleGuideParams{
		ID: g.ID, Name: g.Name, Fields: fields, Rules: rules, Version: int32Of(g.Version), UpdatedBy: g.UpdatedBy,
		UpdatedAt: g.UpdatedAt, ExpectedVersion: int32Of(expected),
	})
	if err != nil {
		return storeError(err)
	}
	if n == 0 {
		return app.ErrStaleVersion
	}
	return nil
}

func (s *store) DeleteStyleGuide(ctx context.Context, id uuid.UUID) error {
	return storeError(s.q.DeleteStyleGuide(ctx, id))
}

func (s *store) StyleGuide(ctx context.Context, id uuid.UUID) (domain.StyleGuide, error) {
	r, err := s.q.GetStyleGuide(ctx, id)
	if err != nil {
		return domain.StyleGuide{}, storeError(err)
	}
	return styleGuide(r)
}

func (s *store) LockStyleGuide(ctx context.Context, id uuid.UUID) (domain.StyleGuide, error) {
	r, err := s.q.LockStyleGuide(ctx, id)
	if err != nil {
		return domain.StyleGuide{}, storeError(err)
	}
	return styleGuide(r)
}

func (s *store) StyleGuides(ctx context.Context, f app.StyleFilter, after uuid.UUID, limit int) ([]domain.StyleGuide, error) {
	rows, err := s.q.ListStyleGuides(ctx, knowledgesql.ListStyleGuidesParams{
		After: after, TenantOnly: f.TenantOnly, ProjectID: nullUUID(f.ProjectID), Locale: nullTag(f.Locale),
		MaxRows: int32Of(limit),
	})
	if err != nil {
		return nil, storeError(err)
	}
	return styleGuides(rows)
}

func (s *store) StyleGuidesInScope(ctx context.Context, project *uuid.UUID) ([]domain.StyleGuide, error) {
	rows, err := s.q.StyleGuidesInScope(ctx, nullUUID(project))
	if err != nil {
		return nil, storeError(err)
	}
	return styleGuides(rows)
}

func (s *store) AppendStyleGuideVersion(ctx context.Context, v app.StyleGuideVersion) error {
	g := v.Guide
	fields, rules, err := encodeStyle(g.StyleContent)
	if err != nil {
		return err
	}
	return storeError(s.q.InsertStyleGuideVersion(ctx, knowledgesql.InsertStyleGuideVersionParams{
		StyleGuideID: g.ID, Version: int32Of(g.Version), Action: string(v.Action), ProjectID: nullUUID(g.Scope.ProjectID),
		Locale: nullTag(g.Scope.Locale), Namespace: nullText(g.Scope.Namespace), Name: g.Name, Fields: fields,
		Rules: rules, Author: v.Author, CreatedAt: v.CreatedAt,
	}))
}

func (s *store) StyleGuideVersions(ctx context.Context, id uuid.UUID, before, limit int) ([]app.StyleGuideVersion, error) {
	rows, err := s.q.ListStyleGuideVersions(ctx, knowledgesql.ListStyleGuideVersionsParams{
		StyleGuideID: id, Before: int32Of(before), MaxRows: int32Of(limit),
	})
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]app.StyleGuideVersion, 0, len(rows))
	for _, r := range rows {
		sc, err := styleScope(r.ProjectID, r.Locale, r.Namespace)
		if err != nil {
			return nil, err
		}
		content, err := styleContent(r.Fields, r.Rules)
		if err != nil {
			return nil, err
		}
		out = append(out, app.StyleGuideVersion{
			Guide: domain.StyleGuide{
				ID: r.StyleGuideID, Scope: sc, Name: r.Name, StyleContent: content, Version: int(r.Version),
				UpdatedBy: r.Author, UpdatedAt: r.CreatedAt.UTC(),
			},
			Action: app.RevisionAction(r.Action), Author: r.Author, CreatedAt: r.CreatedAt.UTC(),
		})
	}
	return out, nil
}

// ── lifecycle ───────────────────────────────────────────────────────

func (s *store) DeleteProjectData(ctx context.Context, project uuid.UUID) error {
	p := uuid.NullUUID{UUID: project, Valid: true}
	for _, del := range []func() error{
		func() error { return s.q.DeleteProjectTM(ctx, p) },
		func() error { return s.q.DeleteProjectDerivations(ctx, project) },
		func() error { return s.q.DeleteProjectConcepts(ctx, p) },
		func() error { return s.q.DeleteProjectConceptRevisions(ctx, p) },
		func() error { return s.q.DeleteProjectStyleGuides(ctx, p) },
		func() error { return s.q.DeleteProjectStyleGuideVersions(ctx, p) },
	} {
		if err := del(); err != nil {
			return storeError(err)
		}
	}
	return nil
}

func (s *store) Publish(ctx context.Context, e outbox.Event) error {
	_, err := outbox.Publish(ctx, s.tx, e)
	return err
}
