// Package translationapp wires translation use cases.
package translationapp

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/translation"
)

// ErrInvalidIDs is the belt-and-suspenders guard against nil-UUID
// inputs slipping through from a broken handler.
var ErrInvalidIDs = errors.New("translationapp: keyId, localeId, and updatedBy must all be non-nil")

// UpdateInput captures the translator-edit shape.
type UpdateInput struct {
	KeyID     uuid.UUID
	LocaleID  uuid.UUID
	Value     string
	Status    string
	UpdatedBy uuid.UUID
}

// UpdateTranslation is the translator-edit use case. Status defaults
// to `needs_review` when omitted — translator saves a draft, an
// editor flips to `approved` afterwards.
type UpdateTranslation struct {
	repo translation.Repository
}

// NewUpdateTranslation wires the use case.
func NewUpdateTranslation(repo translation.Repository) *UpdateTranslation {
	return &UpdateTranslation{repo: repo}
}

// Execute validates input and upserts the translation.
func (uc *UpdateTranslation) Execute(ctx context.Context, in UpdateInput) (translation.Translation, error) {
	if in.KeyID == uuid.Nil || in.LocaleID == uuid.Nil || in.UpdatedBy == uuid.Nil {
		return translation.Translation{}, ErrInvalidIDs
	}
	status := translation.Status(in.Status)
	if in.Status == "" {
		status = translation.StatusNeedsReview
	} else if !status.IsValid() {
		return translation.Translation{}, translation.ErrInvalidStatus
	}
	return uc.repo.Upsert(ctx, translation.Translation{
		KeyID:     in.KeyID,
		LocaleID:  in.LocaleID,
		Value:     in.Value,
		Status:    status,
		UpdatedBy: in.UpdatedBy,
	})
}

// ListBundle is the (project, locale) bundle reader. Used by the SDK
// at runtime and the CLI at build time. Both consumers want the
// full key set including untranslated keys (rendered as empty
// strings) so they can warn or fall back.
type ListBundle struct {
	repo translation.Repository
}

// NewListBundle wires the bundle reader.
func NewListBundle(repo translation.Repository) *ListBundle {
	return &ListBundle{repo: repo}
}

// Execute returns every key for the project paired with its
// translation in the given locale (or an empty value if untranslated).
func (uc *ListBundle) Execute(ctx context.Context, projectID, localeID uuid.UUID) ([]translation.BundleEntry, error) {
	if projectID == uuid.Nil || localeID == uuid.Nil {
		return nil, ErrInvalidIDs
	}
	return uc.repo.ListBundle(ctx, projectID, localeID)
}
