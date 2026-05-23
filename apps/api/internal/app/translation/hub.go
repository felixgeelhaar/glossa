package translationapp

import (
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// Event is the wire shape of a translation broadcast. Mirrors the
// `translation.updated` envelope in docs/design.md §5.2: the SDK,
// the admin UI, and any third-party consumer all key off these
// exact field names. Field order is fixed to keep the JSON
// rendering byte-stable for test assertions.
type Event struct {
	// ID is monotonic per-process. The SSE handler writes it into
	// the `id:` line so reconnecting clients can pass it back via
	// the `Last-Event-ID` header and resume without gaps.
	ID uint64 `json:"-"`

	Type    string `json:"type"`    // always "translation.updated"
	Project string `json:"project"` // project slug
	Locale  string `json:"locale"`  // BCP-47 locale code
	Key     string `json:"key"`     // dotted key name
	Value   string `json:"value"`
	Status  string `json:"status"`
}

// Subscriber holds the per-client channel the hub fans events into.
// Buffered so a slow client can't stall the publisher; on overflow
// the subscriber is dropped (the SDK auto-reconnects with
// Last-Event-ID and replays missed events from the ring).
type Subscriber struct {
	C chan Event
}

// historyDepth caps how far back a reconnecting client can recover.
// 256 events per project is enough for a multi-second outage at
// typical translator-edit rates; if we ever need durable replay we
// move to Redis Streams.
const historyDepth = 256

// Hub is the in-process pub/sub used by the SSE handler. Single
// instance lives in main.go and is shared between the
// translation-update HTTP handler (publisher) and the SSE handler
// (subscriber side).
//
// Replace with a Redis-backed implementation when we go
// multi-replica — the Publisher interface in this file already
// names the seam.
type Hub struct {
	mu      sync.RWMutex
	subs    map[uuid.UUID]map[*Subscriber]struct{}
	history map[uuid.UUID][]Event
	seq     atomic.Uint64
	limiter *tenantLimiter // nil = unlimited; set by NewHubRateLimited
}

// NewHub constructs an empty hub.
func NewHub() *Hub {
	return &Hub{
		subs:    map[uuid.UUID]map[*Subscriber]struct{}{},
		history: map[uuid.UUID][]Event{},
	}
}

// Publisher is the seam the UpdateTranslation flow writes through.
// Carries tenantID so the hub can enforce per-tenant rate limits;
// the HTTP handler reads it off the gin context (set by either
// auth middleware) and passes it in.
type Publisher interface {
	Publish(projectID uuid.UUID, tenantID uuid.UUID, e Event)
}

// Publish broadcasts e to every current subscriber of projectID
// and records the event in the per-project ring for Last-Event-ID
// replay.
//
// Per-tenant rate limit: if the hub was constructed via
// [NewHubRateLimited] and the tenant's bucket is empty, the
// publish is dropped silently. SSE subscribers will still
// reconnect with Last-Event-ID on the next allowed event and
// pick up the trail — they won't see the dropped value, but the
// rate-limit cap is doing its job by definition.
//
// Non-blocking on slow subscribers: a sub whose 32-event buffer
// fills gets dropped + closed; clients reconnect via SDK backoff.
func (h *Hub) Publish(projectID uuid.UUID, tenantID uuid.UUID, e Event) {
	if !h.publishAllowed(tenantID) {
		return
	}
	e.ID = h.seq.Add(1)

	h.mu.Lock()
	defer h.mu.Unlock()

	hist := h.history[projectID]
	hist = append(hist, e)
	if len(hist) > historyDepth {
		hist = hist[len(hist)-historyDepth:]
	}
	h.history[projectID] = hist

	for s := range h.subs[projectID] {
		select {
		case s.C <- e:
		default:
			delete(h.subs[projectID], s)
			close(s.C)
		}
	}
}

// Subscribe registers s for events on projectID and replays any
// events with ID > lastEventID from the history ring. Pass 0 to
// skip replay (fresh connections). The replayed events arrive on
// s.C before any newly-published events, in order.
//
// Caller must invoke Unsubscribe (typically via defer) to free the
// slot — otherwise a closed connection leaks until the next
// drop-on-overflow cycle.
func (h *Hub) Subscribe(projectID uuid.UUID, lastEventID uint64) *Subscriber {
	s := &Subscriber{C: make(chan Event, 32)}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.subs[projectID] == nil {
		h.subs[projectID] = map[*Subscriber]struct{}{}
	}
	h.subs[projectID][s] = struct{}{}

	if lastEventID > 0 {
		for _, e := range h.history[projectID] {
			if e.ID > lastEventID {
				// Buffered channel is fresh and sized to 32; replay
				// happens before any new publishes, so we can send
				// directly without the drop dance.
				s.C <- e
			}
		}
	}
	return s
}

// Unsubscribe removes s from projectID's subscriber set and closes
// the channel. Safe to call multiple times.
func (h *Hub) Unsubscribe(projectID uuid.UUID, s *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[projectID][s]; !ok {
		return
	}
	delete(h.subs[projectID], s)
	close(s.C)
}
