// Package translation owns the Translation aggregate (one
// (key, locale, value) tuple).
package translation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidStatus is returned when a status string is outside the
// schema-enforced enum.
var ErrInvalidStatus = errors.New("translation: status must be one of pending|ai_translated|needs_review|approved")

// Status is the editorial lifecycle of a translation.
type Status string

// Status values. Mirrors the CHECK constraint on the translations
// table. StatusAITranslated marks a row produced by an AI translator
// agent — distinct from human-authored pending and from
// human-reviewed.
const (
	StatusPending      Status = "pending"
	StatusAITranslated Status = "ai_translated"
	StatusNeedsReview  Status = "needs_review"
	StatusApproved     Status = "approved"
)

// IsValid reports whether the status is one of the enum members.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusAITranslated, StatusNeedsReview, StatusApproved:
		return true
	}
	return false
}

// ParseStatus validates a raw status string from the wire.
func ParseStatus(s string) (Status, error) {
	v := Status(s)
	if !v.IsValid() {
		return "", ErrInvalidStatus
	}
	return v, nil
}

// Translation is one (key, locale) → value tuple.
type Translation struct {
	ID        uuid.UUID
	KeyID     uuid.UUID
	LocaleID  uuid.UUID
	Value     string
	Status    Status
	UpdatedBy uuid.UUID
	UpdatedAt time.Time
}

// BundleEntry is one row of a (project, locale) bundle export.
// `Value` is empty + `Status` is the zero value when no translation
// exists yet for the key.
type BundleEntry struct {
	Key    string
	Value  string
	Status Status
}

// ErrNotFound is returned by [Repository.Find] when no row exists
// at the given (key, locale). Distinct from a generic "no rows"
// error so callers can branch on "first translation here" without
// matching the driver's sentinel.
var ErrNotFound = errors.New("translation: not found")

// Repository is the persistence port.
type Repository interface {
	Upsert(ctx context.Context, t Translation) (Translation, error)
	ListBundle(ctx context.Context, projectID, localeID uuid.UUID) ([]BundleEntry, error)
	Find(ctx context.Context, keyID, localeID uuid.UUID) (Translation, error)
}
