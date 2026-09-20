package outbox

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
)

// ErrInvalidSubscription is returned by Subscribe for a bad name or a
// duplicate subscriber.
var ErrInvalidSubscription = errors.New("outbox: invalid subscription")

// Handler handles one delivery. See the package doc for the contract:
// idempotent on EventID, tenant-scoped context, at-least-once.
type Handler interface {
	HandleEvent(ctx context.Context, d Delivery) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, d Delivery) error

// HandleEvent calls f.
func (f HandlerFunc) HandleEvent(ctx context.Context, d Delivery) error { return f(ctx, d) }

type subscription struct {
	name    string
	handler Handler
}

// Registry maps event types to their subscribers.
type Registry struct {
	mu   sync.RWMutex
	subs map[string][]subscription
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{subs: map[string][]subscription{}} }

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

// Subscribe registers h for eventType under a subscriber name, such as
// "localization.mark_outdated". The name is persisted with each event
// to record which subscribers already succeeded, so keep it stable
// across releases; renaming it makes pending events run it again.
func (r *Registry) Subscribe(eventType, subscriber string, h Handler) error {
	if !namePattern.MatchString(eventType) || !namePattern.MatchString(subscriber) || h == nil {
		return fmt.Errorf("%w: type %q, subscriber %q", ErrInvalidSubscription, eventType, subscriber)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.subs[eventType] {
		if s.name == subscriber {
			return fmt.Errorf("%w: %s already subscribed to %s", ErrInvalidSubscription, subscriber, eventType)
		}
	}
	r.subs[eventType] = append(r.subs[eventType], subscription{name: subscriber, handler: h})
	return nil
}

func (r *Registry) subscribers(eventType string) []subscription {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]subscription(nil), r.subs[eventType]...)
}
