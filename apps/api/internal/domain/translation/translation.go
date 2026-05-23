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
var ErrInvalidStatus = errors.New("translation: status must be one of pending|needs_review|approved")

// Status is the editorial lifecycle of a translation.
type Status string

// Status values. Mirrors the CHECK constraint on the translations
// table.
const (
	StatusPending     Status = "pending"
	StatusNeedsReview Status = "needs_review"
	StatusApproved    Status = "approved"
)

// IsValid reports whether the status is one of the enum members.
func (s Status) IsValid() bool {
	return s == StatusPending || s == StatusNeedsReview || s == StatusApproved
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

// Repository is the persistence port.
type Repository interface {
	Upsert(ctx context.Context, t Translation) (Translation, error)
	ListBundle(ctx context.Context, projectID, localeID uuid.UUID) ([]BundleEntry, error)
}
