// Package locale owns the Locale aggregate.
package locale

import (
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"
)

// ErrInvalidCode is returned when a code fails [CodePattern].
var ErrInvalidCode = errors.New("locale: code must be a BCP-47 subtag, e.g. 'de' or 'de-DE'")

// ErrInvalidLabel is returned when a label is empty or oversize.
var ErrInvalidLabel = errors.New("locale: label must be 1-50 characters")

// CodePattern is a deliberately loose BCP-47 check — enough to catch
// typos without re-implementing RFC 5646. The admin UI surfaces a
// dropdown of known locales; freeform entry is the exception.
var CodePattern = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Z]{2})?$`)

// Code is a validated BCP-47 locale subtag.
type Code string

// NewCode parses and validates a code literal.
func NewCode(s string) (Code, error) {
	if !CodePattern.MatchString(s) {
		return "", ErrInvalidCode
	}
	return Code(s), nil
}

// String returns the underlying code value.
func (c Code) String() string { return string(c) }

// Label is a validated display label (e.g. "Deutsch", "English (US)").
type Label string

// NewLabel parses and validates a label literal.
func NewLabel(s string) (Label, error) {
	if l := len(s); l == 0 || l > 50 {
		return "", ErrInvalidLabel
	}
	return Label(s), nil
}

// String returns the underlying label value.
func (l Label) String() string { return string(l) }

// Locale is a single (project, language) combination.
type Locale struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Code      Code
	Label     Label
	Enabled   bool
}

// Repository is the persistence port.
type Repository interface {
	Save(ctx context.Context, l Locale) error
	ListForProject(ctx context.Context, projectID uuid.UUID) ([]Locale, error)
	SetEnabled(ctx context.Context, id uuid.UUID, enabled bool) error
}
