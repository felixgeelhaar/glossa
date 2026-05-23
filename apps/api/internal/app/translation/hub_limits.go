package translationapp

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// HubLimits configures the per-tenant publish rate limit. Zero
// means "no limit" — useful for tests; production wiring in
// cmd/api supplies non-zero values.
type HubLimits struct {
	PerTenantPerSecond float64
	PerTenantBurst     int
}

// NewHubRateLimited wraps NewHub with a per-tenant token bucket on
// the publish path. A runaway project on tenant X can no longer
// blast every SSE subscriber on tenant Y — the limiter pre-empts
// publishes that exceed the budget by dropping them (and emitting
// a metric, future scope).
func NewHubRateLimited(limits HubLimits) *Hub {
	h := NewHub()
	h.limiter = newTenantLimiter(limits)
	return h
}

// publishAllowed is called by Publish before the broadcast. The
// hub passes the project's tenantID; default Hubs (no limiter)
// always allow.
func (h *Hub) publishAllowed(tenantID uuid.UUID) bool {
	if h.limiter == nil {
		return true
	}
	return h.limiter.allow(tenantID)
}

// ─── token-bucket limiter ────────────────────────────────────────

type tenantLimiter struct {
	limits HubLimits
	mu     sync.Mutex
	state  map[uuid.UUID]*bucket
}

type bucket struct {
	tokens   float64
	lastFill time.Time
}

func newTenantLimiter(limits HubLimits) *tenantLimiter {
	if limits.PerTenantPerSecond <= 0 {
		return nil
	}
	if limits.PerTenantBurst <= 0 {
		limits.PerTenantBurst = int(limits.PerTenantPerSecond) * 2
	}
	return &tenantLimiter{limits: limits, state: map[uuid.UUID]*bucket{}}
}

func (l *tenantLimiter) allow(tenantID uuid.UUID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.state[tenantID]
	now := time.Now()
	if !ok {
		b = &bucket{tokens: float64(l.limits.PerTenantBurst), lastFill: now}
		l.state[tenantID] = b
	}
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * l.limits.PerTenantPerSecond
	if b.tokens > float64(l.limits.PerTenantBurst) {
		b.tokens = float64(l.limits.PerTenantBurst)
	}
	b.lastFill = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
