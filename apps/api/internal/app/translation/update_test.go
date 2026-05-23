package translationapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	translationapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translation"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/translation"
)

type inMemoryRepo struct {
	upserted translation.Translation
}

func (r *inMemoryRepo) Upsert(_ context.Context, t translation.Translation) (translation.Translation, error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	r.upserted = t
	return t, nil
}

func (r *inMemoryRepo) ListBundle(_ context.Context, _, _ uuid.UUID) ([]translation.BundleEntry, error) {
	return nil, errors.New("unused in these tests")
}

func TestUpdateTranslation_DefaultsStatusToNeedsReview(t *testing.T) {
	repo := &inMemoryRepo{}
	uc := translationapp.NewUpdateTranslation(repo)
	out, err := uc.Execute(context.Background(), translationapp.UpdateInput{
		KeyID:     uuid.New(),
		LocaleID:  uuid.New(),
		Value:     "Freigeben",
		UpdatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Status != translation.StatusNeedsReview {
		t.Errorf("expected default status needs_review, got %q", out.Status)
	}
}

func TestUpdateTranslation_PassesThroughExplicitStatus(t *testing.T) {
	repo := &inMemoryRepo{}
	uc := translationapp.NewUpdateTranslation(repo)
	out, err := uc.Execute(context.Background(), translationapp.UpdateInput{
		KeyID:     uuid.New(),
		LocaleID:  uuid.New(),
		Value:     "Freigeben",
		Status:    string(translation.StatusApproved),
		UpdatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Status != translation.StatusApproved {
		t.Errorf("expected approved, got %q", out.Status)
	}
}

func TestUpdateTranslation_RejectsInvalidStatus(t *testing.T) {
	repo := &inMemoryRepo{}
	uc := translationapp.NewUpdateTranslation(repo)
	_, err := uc.Execute(context.Background(), translationapp.UpdateInput{
		KeyID:     uuid.New(),
		LocaleID:  uuid.New(),
		Value:     "x",
		Status:    "yolo",
		UpdatedBy: uuid.New(),
	})
	if !errors.Is(err, translation.ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}
}

func TestUpdateTranslation_RejectsZeroIDs(t *testing.T) {
	repo := &inMemoryRepo{}
	uc := translationapp.NewUpdateTranslation(repo)
	_, err := uc.Execute(context.Background(), translationapp.UpdateInput{})
	if !errors.Is(err, translationapp.ErrInvalidIDs) {
		t.Fatalf("expected ErrInvalidIDs, got %v", err)
	}
}
