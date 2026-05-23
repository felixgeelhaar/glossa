package projectapp

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

// ErrInvalidProjectID guards against nil-UUID inputs.
var ErrInvalidProjectID = errors.New("projectapp: project_id required")

// KeyRotator is the narrow port [RotateAPIKey] needs. Implementations
// must persist the new SHA-256 hash atomically; the raw key is the
// caller's responsibility to surface exactly once.
type KeyRotator interface {
	RotateAPIKeyHash(ctx context.Context, id uuid.UUID, hash []byte) error
}

// RotateAPIKey generates a fresh raw key + hash + persists the hash.
// Existing consumers using the old key get 401s on their next request.
type RotateAPIKey struct {
	repo KeyRotator
}

// NewRotateAPIKey wires the use case.
func NewRotateAPIKey(repo KeyRotator) *RotateAPIKey {
	return &RotateAPIKey{repo: repo}
}

// Execute generates a new key, persists its hash, returns the raw
// key for the caller to display once.
func (uc *RotateAPIKey) Execute(ctx context.Context, projectID uuid.UUID) (string, error) {
	if projectID == uuid.Nil {
		return "", ErrInvalidProjectID
	}
	raw, hash, err := generateAPIKey()
	if err != nil {
		return "", err
	}
	if err := uc.repo.RotateAPIKeyHash(ctx, projectID, hash); err != nil {
		return "", err
	}
	return raw, nil
}

// Ensure the Repository interface is satisfied by the standard
// sqlcadapter repo; compile-time guard.
var _ KeyRotator = (project.Repository)(nil)
