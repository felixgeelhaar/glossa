// Package outbox is glossa-server's transactional outbox (RFC 0002 §4).
//
// A bounded context records a domain event with [Publish] inside the
// same tenant-scoped transaction as the state change that raised it, so
// the event exists if and only if the change committed. The
// [Dispatcher] then claims due events in batches (FOR UPDATE SKIP
// LOCKED, so replicas share the work) and delivers each one to the
// in-process handlers subscribed to its type.
//
// # Delivery contract
//
//   - At least once. A handler can see the same event more than once:
//     after a crash, an expired lease, or a failed sibling handler's
//     retry racing a lease. Handlers MUST be idempotent, keyed on
//     [Delivery.EventID] (e.g. a processed-events table or an
//     idempotent upsert in the handler's own transaction).
//   - Per subscriber. A retry re-runs only the subscribers that have
//     not succeeded yet.
//   - Tenant-scoped. The handler's context carries the event's tenant,
//     so db.UnitOfWork.InTenantTx works as it does in a request.
//   - Traced. The publisher's trace context travels with the event, so
//     one trace spans API write → outbox → handler.
//   - No ordering guarantee, not even per aggregate.
//
// # Failure handling
//
// A failing handler is retried in-process a few times (fortify retry
// with jittered exponential backoff). If it still fails, the event is
// rescheduled with a persistent exponential backoff; after MaxAttempts
// deliveries it moves to the dead-letter state (status 'dead') for an
// operator to inspect. Return [Permanent] for errors a retry cannot fix
// (an undecodable payload) to dead-letter immediately.
package outbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// ErrInvalidEvent is returned by Publish for an incomplete event.
var ErrInvalidEvent = errors.New("outbox: invalid event")

// Event is a domain event to publish.
type Event struct {
	// ID identifies the event; a UUIDv7 is generated when zero.
	ID uuid.UUID
	// Type names the event "<context>.<aggregate>.<past-tense verb>" in
	// snake_case, e.g. "identity.member.added" (platform/README.md,
	// "Domain events"). Handlers subscribe by type.
	Type string
	// AggregateType and AggregateID identify the aggregate that raised it.
	AggregateType string
	AggregateID   string
	// Payload is marshalled to JSON (json.RawMessage passes through).
	Payload any
	// OccurredAt defaults to the publish time.
	OccurredAt time.Time
}

const maxNameLen = 200

func (e Event) validate() error {
	for field, v := range map[string]string{
		"Type": e.Type, "AggregateType": e.AggregateType, "AggregateID": e.AggregateID,
	} {
		if v == "" || len(v) > maxNameLen {
			return fmt.Errorf("%w: %s must be 1–%d bytes", ErrInvalidEvent, field, maxNameLen)
		}
	}
	return nil
}

// Delivery is an event as a handler receives it.
type Delivery struct {
	EventID       uuid.UUID
	TenantID      tenancy.ID
	Type          string
	AggregateType string
	AggregateID   string
	Payload       json.RawMessage
	OccurredAt    time.Time
	// Attempt counts deliveries of this event, starting at 1.
	Attempt int
}

// Decode unmarshals the payload into v.
func (d Delivery) Decode(v any) error {
	if err := json.Unmarshal(d.Payload, v); err != nil {
		return Permanent(fmt.Errorf("outbox: decode %s payload: %w", d.Type, err))
	}
	return nil
}

type permanentError struct{ err error }

func (p permanentError) Error() string { return p.err.Error() }
func (p permanentError) Unwrap() error { return p.err }

// Permanent marks err as not worth retrying: the event is dead-lettered
// right away instead of backing off.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err}
}

// IsPermanent reports whether err was marked with Permanent.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}
