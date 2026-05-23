// Package projectapp wires project use cases. Application layer
// depends on the domain ports only; never reaches into infra or
// HTTP.
package projectapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

// ErrInvalidTenantID is the belt-and-suspender guard. Authenticated
// middleware should make this impossible, but use cases stay safe
// even when called from a broken handler.
var ErrInvalidTenantID = errors.New("projectapp: tenant_id required")

// CreateInput captures the project-create request shape. Slug/name
// come from the HTTP layer as raw strings; the use case turns them
// into validated VOs. DefaultLocale is a BCP-47 tag the admin UI
// surfaces as a dropdown — validation lives at the locale boundary
// where it belongs.
type CreateInput struct {
	TenantID      uuid.UUID
	Slug          string
	Name          string
	DefaultLocale string
}

// CreateOutput pairs the persisted Project with the freshly generated
// raw API key. The raw key is the only artifact returned in cleartext
// — only its SHA-256 hash hits the database. Callers MUST surface
// the raw key to the human exactly once and then discard it.
type CreateOutput struct {
	Project   project.Project
	APIKeyRaw string
}

// CreateProject is the use case that creates a new project under a
// tenant. Reusing IRI/Brotwerk's hex-arch shape: tiny struct, single
// Execute method, Repository injected at the boundary.
type CreateProject struct {
	repo project.Repository
}

// NewCreateProject wires the use case.
func NewCreateProject(repo project.Repository) *CreateProject {
	return &CreateProject{repo: repo}
}

// Execute validates input, generates an API key, and persists the
// project. The raw API key is returned alongside the project; the
// hash is stored.
func (uc *CreateProject) Execute(ctx context.Context, in CreateInput) (CreateOutput, error) {
	if in.TenantID == uuid.Nil {
		return CreateOutput{}, ErrInvalidTenantID
	}
	slug, err := project.NewSlug(in.Slug)
	if err != nil {
		return CreateOutput{}, err
	}
	name, err := project.NewName(in.Name)
	if err != nil {
		return CreateOutput{}, err
	}
	defaultLocale := in.DefaultLocale
	if defaultLocale == "" {
		defaultLocale = "de"
	}

	raw, hash, err := generateAPIKey()
	if err != nil {
		return CreateOutput{}, err
	}

	p := project.Project{
		ID:            uuid.New(),
		TenantID:      in.TenantID,
		Slug:          slug,
		Name:          name,
		DefaultLocale: defaultLocale,
		APIKeyHash:    hash,
	}
	if err := uc.repo.Save(ctx, p); err != nil {
		return CreateOutput{}, err
	}
	return CreateOutput{Project: p, APIKeyRaw: raw}, nil
}

// generateAPIKey returns (raw, sha256hash). The raw key carries a
// `glossa_` prefix so a leaked one is grep-able in customer logs.
func generateAPIKey() (string, []byte, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", nil, err
	}
	raw := "glossa_" + hex.EncodeToString(b[:])
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}
