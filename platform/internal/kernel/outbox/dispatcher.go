package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.klarlabs.de/fortify/retry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// DispatcherConfig tunes delivery. Zero values are invalid except where
// noted; config.Outbox supplies production defaults.
type DispatcherConfig struct {
	// BatchSize is the number of events claimed per round trip.
	BatchSize int
	// MaxAttempts is the number of deliveries before dead-lettering.
	MaxAttempts int
	// PollInterval is the idle wait between empty claims.
	PollInterval time.Duration
	// Lease is how long a claim stays exclusive. Events a batch hasn't
	// started by the end of its lease are released, not delivered late.
	Lease time.Duration
	// HandlerTimeout bounds one subscriber's delivery, inline retries
	// included. It must be shorter than Lease.
	HandlerTimeout time.Duration
	// InlineAttempts and InlineBackoff configure the in-process fortify
	// retry per subscriber (defaults: 3 attempts, 100ms).
	InlineAttempts int
	InlineBackoff  time.Duration
	// RetryBase and RetryMax shape the persistent backoff between
	// deliveries: RetryBase·2^(attempt−1), capped at RetryMax
	// (defaults: 5s, 15m).
	RetryBase time.Duration
	RetryMax  time.Duration
}

func (c *DispatcherConfig) applyDefaults() {
	if c.InlineAttempts == 0 {
		c.InlineAttempts = 3
	}
	if c.InlineBackoff == 0 {
		c.InlineBackoff = 100 * time.Millisecond
	}
	if c.RetryBase == 0 {
		c.RetryBase = 5 * time.Second
	}
	if c.RetryMax == 0 {
		c.RetryMax = 15 * time.Minute
	}
}

func (c DispatcherConfig) validate() error {
	var errs []error
	if c.BatchSize < 1 || c.MaxAttempts < 1 || c.InlineAttempts < 1 {
		errs = append(errs, errors.New("BatchSize, MaxAttempts and InlineAttempts must be positive"))
	}
	if c.PollInterval <= 0 || c.HandlerTimeout <= 0 {
		errs = append(errs, errors.New("PollInterval and HandlerTimeout must be positive"))
	}
	if c.Lease <= c.HandlerTimeout {
		errs = append(errs, errors.New("Lease must be longer than HandlerTimeout"))
	}
	if c.RetryMax < c.RetryBase {
		errs = append(errs, errors.New("RetryMax must be at least RetryBase"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("outbox: invalid dispatcher config: %w", errors.Join(errs...))
	}
	return nil
}

// DispatcherOptions carries optional collaborators; nil means a no-op.
type DispatcherOptions struct {
	Logger         *slog.Logger
	TracerProvider trace.TracerProvider
	Registerer     prometheus.Registerer
	// Now is the clock used for lease bookkeeping (tests).
	Now func() time.Time
}

// Dispatcher claims due events and delivers them to subscribers.
type Dispatcher struct {
	store    Store
	registry *Registry
	cfg      DispatcherConfig
	logger   *slog.Logger
	tracer   trace.Tracer
	metrics  *metrics
	now      func() time.Time
	retry    retry.Retry[struct{}]
}

// NewDispatcher validates cfg and wires the dispatcher.
func NewDispatcher(store Store, reg *Registry, cfg DispatcherConfig, opts DispatcherOptions) (*Dispatcher, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	d := &Dispatcher{store: store, registry: reg, cfg: cfg, logger: opts.Logger, now: opts.Now}
	if d.logger == nil {
		d.logger = slog.New(slog.DiscardHandler)
	}
	if d.now == nil {
		d.now = time.Now
	}
	tp := opts.TracerProvider
	if tp == nil {
		tp = noop.NewTracerProvider()
	}
	d.tracer = tp.Tracer("github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox")
	d.metrics = newMetrics(opts.Registerer)
	d.retry = retry.New[struct{}](retry.Config{
		MaxAttempts:   cfg.InlineAttempts,
		InitialDelay:  cfg.InlineBackoff,
		Multiplier:    2,
		BackoffPolicy: retry.BackoffExponential,
		Jitter:        true,
		IsRetryable:   func(err error) bool { return !IsPermanent(err) },
	})
	return d, nil
}

// Run delivers until ctx is cancelled. It polls every PollInterval when
// idle and immediately when the last batch was full. Claim errors are
// logged and retried on the next tick, so a database blip doesn't stop
// the dispatcher.
func (d *Dispatcher) Run(ctx context.Context) error {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		n, err := d.ProcessBatch(ctx)
		if err != nil && ctx.Err() == nil {
			d.logger.ErrorContext(ctx, "outbox: claim failed", slog.Any("error", err))
		}
		wait := d.cfg.PollInterval
		if err == nil && n == d.cfg.BatchSize {
			wait = 0
		}
		timer.Reset(wait)
	}
}

// ProcessBatch claims one batch and settles every claim in it. It
// returns the number of events claimed.
func (d *Dispatcher) ProcessBatch(ctx context.Context) (int, error) {
	leaseEnd := d.now().Add(d.cfg.Lease - d.cfg.Lease/10) // keep a margin for settling
	claims, err := d.store.Claim(ctx, d.cfg.BatchSize, d.cfg.Lease)
	if err != nil {
		d.metrics.claimErrors.Inc()
		return 0, fmt.Errorf("outbox: claim: %w", err)
	}
	for _, c := range claims {
		s := d.settlementFor(ctx, c, leaseEnd)
		d.settle(ctx, c, s)
	}
	return len(claims), nil
}

// settlementFor delivers c unless the dispatcher must hand it back.
func (d *Dispatcher) settlementFor(ctx context.Context, c Claim, leaseEnd time.Time) Settlement {
	if ctx.Err() != nil || !d.now().Before(leaseEnd) {
		return Settlement{EventID: c.EventID, ClaimToken: c.ClaimToken, Outcome: OutcomeRelease}
	}
	return d.deliver(ctx, c)
}

// deliver runs every pending subscriber of c in the event's tenant scope
// and under the publisher's trace.
func (d *Dispatcher) deliver(ctx context.Context, c Claim) Settlement {
	ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier(c.TraceContext))
	ctx, span := d.tracer.Start(ctx, "outbox deliver "+c.Type,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "glossa.outbox"),
			attribute.String("messaging.message.id", c.EventID.String()),
			attribute.String("glossa.event.type", c.Type),
			attribute.String("glossa.tenant.id", c.TenantID.String()),
			attribute.Int("glossa.event.attempt", c.Attempt),
		))
	defer span.End()
	ctx = tenancy.ContextWithTenant(ctx, c.TenantID)

	delivered := slices.Clone(c.DeliveredTo)
	var failures []error
	for _, sub := range d.registry.subscribers(c.Type) {
		if slices.Contains(delivered, sub.name) {
			continue
		}
		if err := d.invoke(ctx, sub, c.Delivery); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", sub.name, err))
			continue
		}
		delivered = append(delivered, sub.name)
	}
	s := d.decide(c, delivered, failures)
	if len(failures) > 0 {
		span.SetStatus(codes.Error, s.LastError)
	}
	span.SetAttributes(attribute.String("glossa.outbox.outcome", s.Outcome.String()))
	return s
}

// invoke calls one subscriber with inline retries, a timeout and panic
// isolation.
func (d *Dispatcher) invoke(ctx context.Context, sub subscription, del Delivery) error {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, d.cfg.HandlerTimeout)
	defer cancel()
	_, err := d.retry.Execute(ctx, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, callSafely(ctx, sub.handler, del)
	})
	d.metrics.observeHandler(del.Type, sub.name, err, time.Since(start))
	if err != nil {
		d.logger.WarnContext(ctx, "outbox: handler failed",
			slog.String("event_type", del.Type), slog.String("event_id", del.EventID.String()),
			slog.String("subscriber", sub.name), slog.Int("attempt", del.Attempt), slog.Any("error", err))
	}
	return err
}

func callSafely(ctx context.Context, h Handler, del Delivery) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panicked: %v\n%s", p, debug.Stack())
		}
	}()
	return h.HandleEvent(ctx, del)
}

// decide turns delivery results into a settlement.
func (d *Dispatcher) decide(c Claim, delivered []string, failures []error) Settlement {
	s := Settlement{EventID: c.EventID, ClaimToken: c.ClaimToken, DeliveredTo: delivered}
	if len(failures) == 0 {
		s.Outcome = OutcomeDelivered
		return s
	}
	s.LastError = joinErrors(failures)
	if slices.ContainsFunc(failures, IsPermanent) || c.Attempt >= d.cfg.MaxAttempts {
		s.Outcome = OutcomeDead
		return s
	}
	s.Outcome = OutcomeRetry
	s.RetryAfter = retryDelay(c.Attempt, d.cfg.RetryBase, d.cfg.RetryMax)
	return s
}

func joinErrors(errs []error) string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	const maxLen = 4096
	msg := strings.Join(msgs, "; ")
	if len(msg) > maxLen {
		msg = msg[:maxLen]
	}
	return msg
}

// retryDelay is base·2^(attempt−1), capped at max.
func retryDelay(attempt int, base, max time.Duration) time.Duration {
	delay := base
	for i := 1; i < attempt && delay < max; i++ {
		delay *= 2
	}
	return min(delay, max)
}

// settle records s even when ctx is cancelled (shutdown), so finished
// work isn't redelivered needlessly.
func (d *Dispatcher) settle(ctx context.Context, c Claim, s Settlement) {
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err := d.store.Settle(sctx, s)
	switch {
	case errors.Is(err, ErrLeaseLost):
		d.metrics.settlements.WithLabelValues("lease_lost").Inc()
		d.logger.WarnContext(ctx, "outbox: lease lost before settling",
			slog.String("event_id", c.EventID.String()), slog.String("outcome", s.Outcome.String()))
	case err != nil:
		d.metrics.settlements.WithLabelValues("error").Inc()
		d.logger.ErrorContext(ctx, "outbox: settle failed; the lease will expire and redeliver",
			slog.String("event_id", c.EventID.String()), slog.Any("error", err))
	default:
		d.metrics.settlements.WithLabelValues(s.Outcome.String()).Inc()
		if s.Outcome == OutcomeDead {
			d.logger.ErrorContext(ctx, "outbox: event dead-lettered",
				slog.String("event_id", c.EventID.String()), slog.String("event_type", c.Type),
				slog.String("tenant_id", c.TenantID.String()), slog.String("error", s.LastError))
		}
	}
}
