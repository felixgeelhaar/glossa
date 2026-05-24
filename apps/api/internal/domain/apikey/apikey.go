// Package apikey owns the project-API-key aggregate. One project
// can issue many keys; each key carries a scope that gates which
// HTTP verbs the holder can call.
package apikey

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Scope enumerates the access levels we issue. Read keys can only
// hit GET endpoints; write keys also unlock PATCH + POST.
type Scope string

const (
	ScopeRead  Scope = "read"
	ScopeWrite Scope = "write"
)

// IsValid reports whether the value is one of the enum members.
func (s Scope) IsValid() bool {
	return s == ScopeRead || s == ScopeWrite
}

// Allows reports whether a key with this scope may perform an
// operation that requires `required`. A write key satisfies any
// read requirement, but not the other way round.
func (s Scope) Allows(required Scope) bool {
	if required == ScopeRead {
		return s == ScopeRead || s == ScopeWrite
	}
	return s == required
}

// ErrInvalidScope is returned by ParseScope for unknown values.
var ErrInvalidScope = errors.New("apikey: scope must be 'read' or 'write'")

// ParseScope validates a wire value.
func ParseScope(s string) (Scope, error) {
	v := Scope(s)
	if !v.IsValid() {
		return "", ErrInvalidScope
	}
	return v, nil
}

// Key is one row in project_api_keys. The raw key value is never
// stored — only the SHA-256 hash. The reveal-once string lives in
// process memory long enough to ship to the admin SPA.
type Key struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	Scope      Scope
	Label      string
	CreatedAt  time.Time
	LastUsedAt time.Time
	RevokedAt  time.Time // zero when active
}

// IsActive reports whether the key is currently usable.
func (k Key) IsActive() bool { return k.RevokedAt.IsZero() }

// Resolution is the result of a hash lookup — the matched key plus
// just enough project context to build a request scope without a
// second query.
type Resolution struct {
	KeyID         uuid.UUID
	ProjectID     uuid.UUID
	TenantID      uuid.UUID
	ProjectSlug   string
	ProjectName   string
	DefaultLocale string
	Scope         Scope
}

// ErrNotFound is returned when no active key matches the given hash.
var ErrNotFound = errors.New("apikey: not found")

// Repository is the persistence port.
type Repository interface {
	Create(ctx context.Context, projectID uuid.UUID, hash []byte, scope Scope, label string) (Key, error)
	List(ctx context.Context, projectID uuid.UUID) ([]Key, error)
	ResolveByHash(ctx context.Context, hash []byte) (Resolution, error)
	Touch(ctx context.Context, id uuid.UUID) error
	Revoke(ctx context.Context, id uuid.UUID) error
}
