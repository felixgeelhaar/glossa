// Package ratelimit implements the preview's Limiter with a fortify
// token bucket per caller, held in process: each glossa-server instance
// enforces the limit on its own, which bounds the CPU one caller can
// spend on any instance (the limit's purpose) without a shared store.
package ratelimit

import (
	"context"
	"time"

	"go.klarlabs.de/fortify/ratelimit"

	"github.com/felixgeelhaar/glossa/platform/internal/preview/app"
)

// Config is a token bucket per key: Rate tokens per Interval, up to
// Burst at once.
type Config struct {
	Rate     int
	Interval time.Duration
	Burst    int
}

// Default is the preview's documented limit: 10 a second per person or
// token, bursts of up to 120 (a burst of keystrokes, or a page of
// messages rendered at once).
func Default() Config { return Config{Rate: 10, Interval: time.Second, Burst: 120} }

// Limiter implements app.Limiter.
type Limiter struct{ rl ratelimit.RateLimiter }

var _ app.Limiter = (*Limiter)(nil)

// Bounds on the in-memory state: idle callers' buckets expire, and the
// number of callers tracked at once is capped.
const (
	maxKeys  = 100_000
	entryTTL = 10 * time.Minute
)

// New returns a Limiter with cfg.
func New(cfg Config) *Limiter {
	store := ratelimit.NewMemoryStoreWithOptions(
		ratelimit.WithMaxKeys(maxKeys), ratelimit.WithEntryTTL(entryTTL), ratelimit.WithCleanupInterval(time.Minute),
	)
	return &Limiter{rl: ratelimit.New(ratelimit.Config{
		Rate: cfg.Rate, Interval: cfg.Interval, Burst: cfg.Burst, Store: store,
	})}
}

// Allow implements app.Limiter.
func (l *Limiter) Allow(ctx context.Context, key string) bool { return l.rl.Allow(ctx, key) }

// Close stops the store's cleanup.
func (l *Limiter) Close() error { return l.rl.Close() }
