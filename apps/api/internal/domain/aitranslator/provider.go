// Package aitranslator owns the Provider aggregate (a configured LLM
// endpoint for automated translation) and the Translator port the
// upsert pipeline calls into.
package aitranslator

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Kind enumerates supported provider implementations.
type Kind string

const (
	KindOpenAI    Kind = "openai"
	KindAnthropic Kind = "anthropic"
	KindGemini    Kind = "gemini"
	KindCustom    Kind = "custom"
)

// IsValid reports whether the kind is a known implementation.
func (k Kind) IsValid() bool {
	switch k {
	case KindOpenAI, KindAnthropic, KindGemini, KindCustom:
		return true
	}
	return false
}

// ErrInvalidKind is returned by [ParseKind] when the input is not one
// of the enum members.
var ErrInvalidKind = errors.New("aitranslator: kind must be one of openai|anthropic|gemini|custom")

// ParseKind validates a wire string.
func ParseKind(s string) (Kind, error) {
	k := Kind(s)
	if !k.IsValid() {
		return "", ErrInvalidKind
	}
	return k, nil
}

// Provider is the aggregate root: an LLM endpoint a tenant has
// configured. APIKeyCT + APIKeyNonce store the AES-GCM ciphertext +
// nonce of the raw key; the plaintext only ever lives in process
// memory during a request.
type Provider struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Kind        Kind
	Label       string
	BaseURL     string
	Model       string
	APIKeyCT    []byte
	APIKeyNonce []byte
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TranslateRequest is what the pipeline hands the Translator port.
type TranslateRequest struct {
	Key          string
	SourceLocale string
	TargetLocale string
	Source       string
}

// TranslateResult is what the port hands back. Provider records
// which Provider produced the output so audit_log can attribute it.
type TranslateResult struct {
	Translation string
	Provider    string
}

// Translator is the outbound port: anything that can turn a source
// string into a target-locale string. The infra implementation
// chooses a provider per call from the tenant's enabled list.
type Translator interface {
	Translate(ctx context.Context, p Provider, plainKey []byte, req TranslateRequest) (TranslateResult, error)
}

// ErrNotFound is returned by Repository.Get when no row matches.
var ErrNotFound = errors.New("aitranslator: provider not found")

// Repository is the persistence port for Provider.
type Repository interface {
	Create(ctx context.Context, p Provider) (Provider, error)
	List(ctx context.Context, tenantID uuid.UUID) ([]Provider, error)
	ListEnabled(ctx context.Context, tenantID uuid.UUID) ([]Provider, error)
	Get(ctx context.Context, id uuid.UUID) (Provider, error)
	Update(ctx context.Context, id uuid.UUID, label, baseURL, model string, enabled bool) error
	UpdateKey(ctx context.Context, id uuid.UUID, ct, nonce []byte) error
	Delete(ctx context.Context, id uuid.UUID) error
}
