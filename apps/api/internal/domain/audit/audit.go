// Package audit owns the append-only audit log of translation
// mutations. Reads are paginated and tenant-scoped through RLS.
package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Entry is one row in the audit_log table.
type Entry struct {
	ID            int64
	TenantID      uuid.UUID
	TranslationID uuid.UUID
	BeforeValue   string
	AfterValue    string
	ChangedBy     uuid.UUID // uuid.Nil for non-human changes
	ActorKind     string    // "user" (default), "ai", or "system"
	ActorLabel    string    // free-form ("openai", "gemini", "bootstrap")
	ChangedAt     time.Time
}

// Repository persists / lists audit entries.
type Repository interface {
	Append(ctx context.Context, e Entry) error
	ListForTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]Entry, error)
}
