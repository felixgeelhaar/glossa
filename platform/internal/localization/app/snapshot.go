package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
	"github.com/felixgeelhaar/glossa/platform/internal/localization/domain"
)

// MessagesWithCoverage lists the IDs of a project's messages that are
// missing or outdated in a locale, in key order — Catalog's message
// list filters through it. It reads Localization's projection, so a
// message is listed once its catalog event has been handled.
func (s *Service) MessagesWithCoverage(ctx context.Context, project uuid.UUID, f CoverageFilter) ([]uuid.UUID, error) {
	if err := authz.Require(ctx, authz.TranslationsRead); err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		ids, err = st.MessagesWithCoverage(ctx, project, f)
		return err
	})
	return ids, err
}

// TranslationWithSource is a translation together with what it was
// translated from: the message's key and namespace (Localization's
// projection; "" while it hasn't seen the message), the project's
// source locale and the source revision the text was made against.
type TranslationWithSource struct {
	TranslationView
	Key          string
	Namespace    string
	SourceLocale bcp47.Tag
	Source       mfcontent.Content
}

// TranslationWithSource reads a translation of project by ID with its
// source — the read Knowledge derives translation memory from when a
// translation is revised or reviewed. It needs translations.read (and,
// through Catalog's port, catalog.read).
func (s *Service) TranslationWithSource(ctx context.Context, project uuid.UUID, id domain.TranslationID) (TranslationWithSource, error) {
	if err := authz.Require(ctx, authz.TranslationsRead); err != nil {
		return TranslationWithSource{}, err
	}
	var out TranslationWithSource
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		row, key, ns, err := st.TranslationByID(ctx, id)
		if err != nil {
			return err
		}
		if row.ProjectID != project {
			return ErrNotFound
		}
		out = TranslationWithSource{TranslationView: view(row), Key: key, Namespace: ns}
		return nil
	})
	if err != nil {
		return TranslationWithSource{}, err
	}
	p, err := s.catalog.Project(ctx, project)
	if err != nil {
		return TranslationWithSource{}, err
	}
	out.SourceLocale = p.SourceLocale
	if out.Source, err = s.catalog.SourceAt(ctx, project, out.MessageID, out.SourceRevision); err != nil {
		return TranslationWithSource{}, err
	}
	return out, nil
}

// LocaleInfo is a locale as a release manifest lists it.
type LocaleInfo struct {
	Code      bcp47.Tag
	Direction bcp47.Direction
	IsSource  bool
}

// TranslationSnapshot is what a release takes from Localization for one
// project: its locales (with direction), the fallback graph exactly as
// the manifest's `fallback` member, and every translation in an eligible
// review state, keyed by message ID. Runtimes/SPEC.md §1: the Release
// context joins it with Catalog's ReleaseSource (active messages, their
// keys, namespaces and source content — the source locale's artifact),
// keeps translations of active messages in the project's locales, and
// writes one artifact per locale and namespace.
type TranslationSnapshot struct {
	SourceLocale bcp47.Tag
	Locales      []LocaleInfo
	Fallback     map[string][]string
	// Translations maps a locale to message ID to translation.
	Translations map[bcp47.Tag]map[uuid.UUID]TranslationView
}

// ReleaseTranslations reads a project's releasable translations in one
// transaction. states says which review states are eligible (an
// environment policy: production takes approved, preview everything).
// Outdated translations are included and flagged — the policy decides.
func (s *Service) ReleaseTranslations(ctx context.Context, project uuid.UUID, states []domain.ReviewState) (TranslationSnapshot, error) {
	if err := authz.Require(ctx, authz.TranslationsRead); err != nil {
		return TranslationSnapshot{}, err
	}
	p, err := s.catalog.Project(ctx, project)
	if err != nil {
		return TranslationSnapshot{}, err
	}
	snap := TranslationSnapshot{SourceLocale: p.SourceLocale, Translations: map[bcp47.Tag]map[uuid.UUID]TranslationView{}}
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		locales, err := st.AllLocales(ctx, project)
		if err != nil {
			return err
		}
		inProject := map[bcp47.Tag]bool{p.SourceLocale: true}
		snap.Locales = append(snap.Locales, LocaleInfo{Code: p.SourceLocale, Direction: p.SourceLocale.Direction(), IsSource: true})
		for _, l := range locales {
			if l.Code == p.SourceLocale {
				continue
			}
			inProject[l.Code] = true
			snap.Locales = append(snap.Locales, LocaleInfo{Code: l.Code, Direction: l.Direction()})
		}
		g, err := st.FallbackGraph(ctx, project, false)
		if err != nil {
			return err
		}
		snap.Fallback = g.Edges
		rows, err := st.SnapshotTranslations(ctx, project, states)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if !inProject[r.Locale] || r.Locale == p.SourceLocale {
				continue
			}
			if snap.Translations[r.Locale] == nil {
				snap.Translations[r.Locale] = map[uuid.UUID]TranslationView{}
			}
			snap.Translations[r.Locale][r.MessageID] = view(r)
		}
		return nil
	})
	return snap, err
}
