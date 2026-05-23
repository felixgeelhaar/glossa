// Package tenant owns the Tenant aggregate. A tenant is the top-level
// isolation boundary; every other Glossa resource hangs off one tenant
// either directly (audit_log, users) or via a project FK.
package tenant

import (
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"
)

// ErrInvalidSlug is returned for slugs that fail [SlugPattern].
var ErrInvalidSlug = errors.New("tenant: slug must be 1-50 chars, lowercase letters / digits / hyphens")

// ErrInvalidName is returned for empty or oversize names.
var ErrInvalidName = errors.New("tenant: name must be 1-200 characters")

// SlugPattern matches [project.SlugPattern]; tenants and projects
// share the URL-safe-ident shape so a single regex documents both.
var SlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,48}[a-z0-9])?$`)

// Slug is a validated tenant slug.
type Slug string

// NewSlug parses and validates a slug literal.
func NewSlug(s string) (Slug, error) {
	if !SlugPattern.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return Slug(s), nil
}

// String returns the underlying slug.
func (s Slug) String() string { return string(s) }

// Name is a validated display name.
type Name string

// NewName parses and validates a name literal.
func NewName(s string) (Name, error) {
	if l := len(s); l == 0 || l > 200 {
		return "", ErrInvalidName
	}
	return Name(s), nil
}

// String returns the underlying name value.
func (n Name) String() string { return string(n) }

// Tenant is the top-level isolation boundary.
type Tenant struct {
	ID   uuid.UUID
	Slug Slug
	Name Name
}

// Repository is the persistence port for tenants.
type Repository interface {
	Save(ctx context.Context, t Tenant) error
	FindBySlug(ctx context.Context, s Slug) (Tenant, error)
	FindByID(ctx context.Context, id uuid.UUID) (Tenant, error)
}
