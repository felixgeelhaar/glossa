package app

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/localization/domain"
)

// LocaleCounts is one locale's row of the stored summary.
type LocaleCounts struct {
	Code     bcp47.Tag
	IsSource bool
	States   domain.StateCounts
	Outdated int
}

// StoredStats is what one summary query returns: the project's active
// messages and the counts of every locale it has.
type StoredStats struct {
	Messages int
	Locales  []LocaleCounts
}

// LocaleStats is a locale with its coverage.
type LocaleStats struct {
	Code     bcp47.Tag
	IsSource bool
	domain.Coverage
}

// Direction is the locale's text direction.
func (l LocaleStats) Direction() bcp47.Direction { return l.Code.Direction() }

// TranslationStats is a project's per-locale status summary.
type TranslationStats struct {
	// Messages counts the project's active messages.
	Messages int
	// Locales are the project's locales by code, the source included.
	Locales []LocaleStats
}

// TranslationStats summarizes every locale of a project in one query:
// active messages, translated, missing and outdated, and translations
// per review state — Studio's list badges and the CLI's status. It is
// computed from Localization's projection on each call rather than kept
// up to date on every write: a source revision alone changes the
// outdated count of every locale, and an aggregate over the project's
// indexed rows is correct by construction.
func (s *Service) TranslationStats(ctx context.Context, project uuid.UUID) (TranslationStats, error) {
	if err := authz.Require(ctx, authz.TranslationsRead); err != nil {
		return TranslationStats{}, err
	}
	p, err := s.catalog.Project(ctx, project)
	if err != nil {
		return TranslationStats{}, err
	}
	var stored StoredStats
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		stored, err = st.TranslationStats(ctx, project)
		return err
	})
	if err != nil {
		return TranslationStats{}, err
	}
	out := TranslationStats{Messages: stored.Messages}
	sourceSeen := false
	for _, l := range stored.Locales {
		isSource := l.Code == p.SourceLocale
		sourceSeen = sourceSeen || isSource
		out.Locales = append(out.Locales, localeStats(l.Code, isSource, stored.Messages, l))
	}
	// A project seen before its created event was handled still has
	// its source locale.
	if !sourceSeen {
		out.Locales = append(out.Locales, localeStats(p.SourceLocale, true, stored.Messages, LocaleCounts{}))
	}
	// By code, in the same order as the locale list.
	slices.SortFunc(out.Locales, func(a, b LocaleStats) int { return strings.Compare(a.Code.String(), b.Code.String()) })
	return out, nil
}

func localeStats(code bcp47.Tag, isSource bool, messages int, c LocaleCounts) LocaleStats {
	cov := domain.LocaleCoverage(messages, c.States, c.Outdated)
	if isSource {
		cov = domain.SourceCoverage(messages)
	}
	return LocaleStats{Code: code, IsSource: isSource, Coverage: cov}
}
