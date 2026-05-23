// Package translationkey owns the Key aggregate (a translation-key
// id like `coach.plan.approve`). Named translationkey rather than
// `key` to avoid shadowing the language keyword everywhere.
package translationkey

import (
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"
)

// ErrInvalidName is returned when a key name fails [NamePattern].
var ErrInvalidName = errors.New("translationkey: name must be 1-255 chars, dotted lowercase identifiers")

// NamePattern matches dotted lowercase identifiers like
// `coach.plan.approve` or `athlete-dashboard.greeting`. Codebase
// conventions across IRI + Brotwerk consistently use this shape;
// rejecting anything else stops translator typos before the DB.
var NamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,253}[a-z0-9]$|^[a-z0-9]$`)

// Name is a validated translation-key name.
type Name string

// NewName parses and validates a name literal.
func NewName(s string) (Name, error) {
	if !NamePattern.MatchString(s) {
		return "", ErrInvalidName
	}
	return Name(s), nil
}

// String returns the underlying name value.
func (n Name) String() string { return string(n) }

// Key is one translatable identifier in a project. Keys are
// project-scoped; cross-project shared keys arrive via translation
// memory (post-MVP, see docs/design.md §10).
type Key struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	Name        Name
	Description string
}

// Repository is the persistence port.
type Repository interface {
	Upsert(ctx context.Context, k Key) (Key, error)
	ListForProject(ctx context.Context, projectID uuid.UUID) ([]Key, error)
	Find(ctx context.Context, projectID uuid.UUID, name Name) (Key, error)
}
