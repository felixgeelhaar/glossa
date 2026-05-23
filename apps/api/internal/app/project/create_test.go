package projectapp_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	projectapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/project"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

// inMemoryRepo is a test-only adapter so use-case tests don't reach
// the DB. Lives in *_test.go per project convention (mirrors the
// brotwerk apps/api pattern).
type inMemoryRepo struct {
	mu       sync.Mutex
	projects map[uuid.UUID]project.Project
	saveOK   bool
}

func newInMemoryRepo() *inMemoryRepo {
	return &inMemoryRepo{projects: map[uuid.UUID]project.Project{}, saveOK: true}
}

func (r *inMemoryRepo) Save(_ context.Context, p project.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.saveOK {
		return errors.New("simulated repo failure")
	}
	r.projects[p.ID] = p
	return nil
}

func (r *inMemoryRepo) Find(_ context.Context, _ uuid.UUID, _ project.Slug) (project.Project, error) {
	return project.Project{}, errors.New("not used in these tests")
}

func (r *inMemoryRepo) FindByAPIKeyHash(_ context.Context, _ []byte) (project.Project, error) {
	return project.Project{}, errors.New("not used in these tests")
}

func (r *inMemoryRepo) ListForTenant(_ context.Context, _ uuid.UUID) ([]project.Project, error) {
	return nil, errors.New("not used in these tests")
}

func (r *inMemoryRepo) RotateAPIKeyHash(_ context.Context, id uuid.UUID, hash []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.projects[id]
	if !ok {
		return errors.New("project not found")
	}
	p.APIKeyHash = hash
	r.projects[id] = p
	return nil
}

func TestCreateProject_ReturnsRawAPIKeyAndStoresHash(t *testing.T) {
	repo := newInMemoryRepo()
	uc := projectapp.NewCreateProject(repo)

	out, err := uc.Execute(context.Background(), projectapp.CreateInput{
		TenantID:      uuid.New(),
		Slug:          "brotwerk-web",
		Name:          "Brotwerk Web",
		DefaultLocale: "de",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	if !strings.HasPrefix(out.APIKeyRaw, "glossa_") {
		t.Errorf("raw key should be prefixed with glossa_: %q", out.APIKeyRaw)
	}
	if len(out.APIKeyRaw) < 32 {
		t.Errorf("raw key too short: %d", len(out.APIKeyRaw))
	}

	stored, ok := repo.projects[out.Project.ID]
	if !ok {
		t.Fatal("expected project to be persisted")
	}
	if len(stored.APIKeyHash) != 32 {
		t.Errorf("expected SHA-256 hash (32 bytes), got %d", len(stored.APIKeyHash))
	}
	// The stored hash MUST NOT equal the raw key bytes — sanity check
	// that we didn't accidentally store cleartext.
	if string(stored.APIKeyHash) == out.APIKeyRaw {
		t.Error("api_key_hash equals the raw key — cleartext leak!")
	}
}

func TestCreateProject_DefaultsLocaleToDe(t *testing.T) {
	repo := newInMemoryRepo()
	uc := projectapp.NewCreateProject(repo)

	out, err := uc.Execute(context.Background(), projectapp.CreateInput{
		TenantID: uuid.New(),
		Slug:     "x",
		Name:     "X",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Project.DefaultLocale != "de" {
		t.Errorf("expected default locale 'de', got %q", out.Project.DefaultLocale)
	}
}

func TestCreateProject_RejectsBlankTenantID(t *testing.T) {
	repo := newInMemoryRepo()
	uc := projectapp.NewCreateProject(repo)

	_, err := uc.Execute(context.Background(), projectapp.CreateInput{
		Slug: "x",
		Name: "X",
	})
	if !errors.Is(err, projectapp.ErrInvalidTenantID) {
		t.Fatalf("expected ErrInvalidTenantID, got %v", err)
	}
}

func TestCreateProject_RejectsInvalidSlug(t *testing.T) {
	repo := newInMemoryRepo()
	uc := projectapp.NewCreateProject(repo)

	_, err := uc.Execute(context.Background(), projectapp.CreateInput{
		TenantID: uuid.New(),
		Slug:     "Invalid Slug!",
		Name:     "X",
	})
	if !errors.Is(err, project.ErrInvalidSlug) {
		t.Fatalf("expected ErrInvalidSlug, got %v", err)
	}
}

func TestCreateProject_PropagatesRepoFailure(t *testing.T) {
	repo := newInMemoryRepo()
	repo.saveOK = false
	uc := projectapp.NewCreateProject(repo)

	_, err := uc.Execute(context.Background(), projectapp.CreateInput{
		TenantID: uuid.New(),
		Slug:     "x",
		Name:     "X",
	})
	if err == nil {
		t.Fatal("expected repo error to bubble")
	}
}
