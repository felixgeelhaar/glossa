package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/apps/api/internal/app/auth"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/user"
)

// fakeUserRepo is a tiny in-memory implementation. The auth tests
// don't need RLS or pgx — they verify the bcrypt + JWT plumbing.
type fakeUserRepo struct {
	byEmail map[string]user.User
}

func (f *fakeUserRepo) Save(ctx context.Context, u user.User) (user.User, error) { return u, nil }
func (f *fakeUserRepo) FindByEmail(ctx context.Context, tenantID uuid.UUID, email string) (user.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return u, nil
}
func (f *fakeUserRepo) FindByID(ctx context.Context, id uuid.UUID) (user.User, error) {
	return user.User{}, user.ErrNotFound
}
func (f *fakeUserRepo) ListForTenant(ctx context.Context, tenantID uuid.UUID) ([]user.User, error) {
	return nil, nil
}
func (f *fakeUserRepo) UpdateLocales(ctx context.Context, id uuid.UUID, locales []string) error {
	return nil
}
func (f *fakeUserRepo) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash []byte) error {
	return nil
}
func (f *fakeUserRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (f *fakeUserRepo) CountAdmins(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	return 0, nil
}
func (f *fakeUserRepo) ListTenantsForEmail(ctx context.Context, email string) ([]user.TenantMembership, error) {
	return nil, nil
}

func TestHMACIssuer_RoundTrip(t *testing.T) {
	iss, err := auth.NewHMACIssuer([]byte("01234567890123456789012345678901"), "glossa", time.Hour)
	if err != nil {
		t.Fatalf("new issuer: %v", err)
	}
	claims := auth.Claims{
		UserID:   uuid.New(),
		TenantID: uuid.New(),
		Email:    "felix@example.com",
		Role:     user.RoleAdmin,
		Locales:  []string{"de"},
	}
	tok, err := iss.Issue(claims)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	out, err := iss.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if out.UserID != claims.UserID || out.Role != claims.Role || out.Email != claims.Email {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestHMACIssuer_RejectsTamperedToken(t *testing.T) {
	iss, _ := auth.NewHMACIssuer([]byte("01234567890123456789012345678901"), "glossa", time.Hour)
	tok, _ := iss.Issue(auth.Claims{UserID: uuid.New(), TenantID: uuid.New(), Role: user.RoleAdmin})
	// Flip the last char of the signature.
	bad := tok[:len(tok)-1] + "x"
	if _, err := iss.Verify(bad); err == nil {
		t.Fatal("expected verify to reject tampered token")
	}
}

func TestLogin_SuccessIssuesUsableToken(t *testing.T) {
	hash, err := auth.HashPassword("hunter2hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	repo := &fakeUserRepo{byEmail: map[string]user.User{
		"felix@example.com": {
			ID:           uuid.New(),
			TenantID:     uuid.New(),
			Email:        "felix@example.com",
			PasswordHash: hash,
			Role:         user.RoleAdmin,
		},
	}}
	iss, _ := auth.NewHMACIssuer([]byte("01234567890123456789012345678901"), "glossa", time.Hour)
	uc := auth.NewLogin(repo, iss)
	out, err := uc.Execute(context.Background(), auth.LoginInput{
		Email:    "felix@example.com",
		Password: "hunter2hunter2",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	verified, err := iss.Verify(out.Token)
	if err != nil {
		t.Fatalf("issued token unverifiable: %v", err)
	}
	if verified.Email != "felix@example.com" {
		t.Fatalf("claims email: got %q", verified.Email)
	}
}

func TestLogin_RejectsWrongPassword(t *testing.T) {
	hash, _ := auth.HashPassword("hunter2hunter2")
	repo := &fakeUserRepo{byEmail: map[string]user.User{
		"felix@example.com": {ID: uuid.New(), TenantID: uuid.New(), Email: "felix@example.com", PasswordHash: hash, Role: user.RoleAdmin},
	}}
	iss, _ := auth.NewHMACIssuer([]byte("01234567890123456789012345678901"), "glossa", time.Hour)
	uc := auth.NewLogin(repo, iss)
	_, err := uc.Execute(context.Background(), auth.LoginInput{Email: "felix@example.com", Password: "wrong-password"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_RejectsUnknownEmail(t *testing.T) {
	iss, _ := auth.NewHMACIssuer([]byte("01234567890123456789012345678901"), "glossa", time.Hour)
	uc := auth.NewLogin(&fakeUserRepo{byEmail: map[string]user.User{}}, iss)
	_, err := uc.Execute(context.Background(), auth.LoginInput{Email: "ghost@example.com", Password: "anything-long-enough"})
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}
