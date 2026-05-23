package user

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("user: not found")

// Repository is the port — adapter lives in
// internal/infra/sqlcadapter/user_repo.go.
type Repository interface {
	Save(ctx context.Context, u User) (User, error)
	FindByEmail(ctx context.Context, tenantID uuid.UUID, email string) (User, error)
	FindByID(ctx context.Context, id uuid.UUID) (User, error)
	ListForTenant(ctx context.Context, tenantID uuid.UUID) ([]User, error)
	UpdateLocales(ctx context.Context, id uuid.UUID, locales []string) error
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash []byte) error
	Delete(ctx context.Context, id uuid.UUID) error
	CountAdmins(ctx context.Context, tenantID uuid.UUID) (int64, error)
}
