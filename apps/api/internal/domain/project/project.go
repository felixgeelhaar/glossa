// Package project owns the Project aggregate and its value objects.
// Domain types only — no persistence, no HTTP. Use cases in
// internal/app depend on the [Repository] port defined here.
package project

import (
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"
)

// ErrInvalidSlug is returned when a slug fails [SlugPattern].
var ErrInvalidSlug = errors.New("project: slug must be 1-50 chars, lowercase letters / digits / hyphens")

// ErrInvalidName is returned when a name is empty or longer than 200
// chars. Names are display strings — full UTF-8 allowed.
var ErrInvalidName = errors.New("project: name must be 1-200 characters")

// SlugPattern enforces a URL-safe identifier. Mirrors what the admin
// UI surfaces; the schema also constrains via the column type.
var SlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,48}[a-z0-9])?$`)

// Slug is a validated project identifier.
type Slug string

// NewSlug parses and validates a slug literal.
func NewSlug(s string) (Slug, error) {
	if !SlugPattern.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return Slug(s), nil
}

// String returns the underlying slug value.
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

// Project is the root aggregate for a tenant's translation surface.
// Locales, keys, and translations all hang off a Project.
//
// API keys live in their own aggregate (domain/apikey) since v0.2 —
// a project can have any number of read- or write-scoped keys.
type Project struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	Slug          Slug
	Name          Name
	DefaultLocale string // BCP-47 tag; validated by [locale.Code] at the locale boundary
}

// Repository is the persistence port. Implementations live in
// internal/infra/sqlcadapter.
type Repository interface {
	Save(ctx context.Context, p Project) error
	Find(ctx context.Context, tenantID uuid.UUID, slug Slug) (Project, error)
	ListForTenant(ctx context.Context, tenantID uuid.UUID) ([]Project, error)
}
