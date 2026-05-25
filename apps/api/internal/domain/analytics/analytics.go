// Package analytics owns the append-only event ledger that drives
// onboarding-funnel + usage metrics.
package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Kind enumerates the events the use cases emit. First-time variants
// are derived at read-time via MIN(occurred_at); the writer never
// has to ask "is this the first one?".
type Kind string

const (
	KindProjectCreated    Kind = "project_created"
	KindKeySynced         Kind = "key_synced"
	KindTranslationEdited Kind = "translation_edited"
	KindConsumerRequest   Kind = "consumer_request"
	KindAITranslation     Kind = "ai_translation"
)

// Event is one row in analytics_events.
type Event struct {
	TenantID   uuid.UUID
	ProjectID  *uuid.UUID // nil for tenant-wide events
	Kind       Kind
	Metadata   map[string]any
	OccurredAt time.Time
}

// FunnelRow captures one (kind, first_at, total) tuple for a project.
type FunnelRow struct {
	Kind    Kind
	FirstAt time.Time
	Total   int64
}

// ProjectFirstEvent is one (project_id, kind, first_at) row from the
// tenant-wide cohort query.
type ProjectFirstEvent struct {
	ProjectID uuid.UUID
	Kind      Kind
	FirstAt   time.Time
}

// Repository is the persistence port.
type Repository interface {
	Record(ctx context.Context, e Event) error
	ProjectFunnel(ctx context.Context, tenantID, projectID uuid.UUID) ([]FunnelRow, error)
	TenantProjectsFirstEvents(ctx context.Context, tenantID uuid.UUID) ([]ProjectFirstEvent, error)
}

// Recorder is the fire-and-forget port the rest of the code holds —
// narrower than Repository so use cases can't accidentally read.
type Recorder interface {
	Record(ctx context.Context, e Event) error
}
