package projectapp_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	projectapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/project"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

func TestRotateAPIKey_ReplacesStoredHash(t *testing.T) {
	repo := newInMemoryRepo()
	create := projectapp.NewCreateProject(repo, &stubLocaleRepo{})
	rotate := projectapp.NewRotateAPIKey(repo)

	created, err := create.Execute(context.Background(), projectapp.CreateInput{
		TenantID: uuid.New(),
		Slug:     "iri",
		Name:     "IRI",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	oldHash := append([]byte(nil), repo.projects[created.Project.ID].APIKeyHash...)

	newRaw, err := rotate.Execute(context.Background(), created.Project.ID)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}

	stored := repo.projects[created.Project.ID]
	if string(stored.APIKeyHash) == string(oldHash) {
		t.Fatal("hash should change after rotation")
	}
	// The new hash must be the SHA-256 of the new raw key.
	want := sha256.Sum256([]byte(newRaw))
	if string(stored.APIKeyHash) != string(want[:]) {
		t.Fatal("stored hash should match SHA-256 of the new raw key")
	}
	if !strings.HasPrefix(newRaw, "glossa_") {
		t.Errorf("expected glossa_ prefix on rotated key, got %q", newRaw)
	}
}

func TestRotateAPIKey_RejectsBlankProjectID(t *testing.T) {
	repo := newInMemoryRepo()
	rotate := projectapp.NewRotateAPIKey(repo)
	_, err := rotate.Execute(context.Background(), uuid.Nil)
	if !errors.Is(err, projectapp.ErrInvalidProjectID) {
		t.Fatalf("expected ErrInvalidProjectID, got %v", err)
	}
}

// Compile-time guard: domain.Repository satisfies the narrow
// KeyRotator port the use case requires.
var _ projectapp.KeyRotator = (project.Repository)(nil)
