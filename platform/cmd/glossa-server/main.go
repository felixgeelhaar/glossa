// Command glossa-server is Glossa's control plane (RFC 0002 §3). This
// file is the composition root and nothing else: it loads config, builds
// telemetry, opens the database, optionally migrates, and runs the HTTP
// server and the outbox dispatcher until SIGINT/SIGTERM, then drains.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"

	intelligenceapp "github.com/felixgeelhaar/glossa/platform/internal/intelligence/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/config"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/httpserver"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/observability"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.LookupEnv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "glossa-server:", err)
		os.Exit(1)
	}
}

// run is main without process globals, so tests can drive it.
func run(ctx context.Context, args []string, lookup config.LookupFunc, stdout io.Writer) error {
	lookup, showVersion, err := parseFlags(args, lookup)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if showVersion {
		_, err := fmt.Fprintln(stdout, version())
		return err
	}
	cfg, err := config.Load(lookup)
	if err != nil {
		return err
	}
	logger := observability.NewLogger(stdout, cfg.LogLevel, tenantAttrs)
	logger.InfoContext(ctx, "glossa-server starting", slog.String("version", version()), slog.String("config", cfg.String()))

	if cfg.Migrate != config.MigrateOff {
		if err := migrate(ctx, cfg, logger); err != nil || cfg.Migrate == config.MigrateOnly {
			return err
		}
	}
	return serve(ctx, cfg, logger)
}

// parseFlags lets -migrate override GLOSSA_MIGRATE, so the same image
// runs as a migration Job (`-migrate=only`) or a server.
func parseFlags(args []string, lookup config.LookupFunc) (config.LookupFunc, bool, error) {
	fs := flag.NewFlagSet("glossa-server", flag.ContinueOnError)
	migrateMode := fs.String("migrate", "", "off | up | only (overrides GLOSSA_MIGRATE)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return nil, false, err
	}
	if *migrateMode == "" {
		return lookup, *showVersion, nil
	}
	return func(key string) (string, bool) {
		if key == "GLOSSA_MIGRATE" {
			return *migrateMode, true
		}
		return lookup(key)
	}, *showVersion, nil
}

func migrate(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	m, err := db.NewMigrator(cfg.MigrationDatabaseURL.Reveal(), logger)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	if err := m.Up(ctx); err != nil {
		return err
	}
	v, dirty, err := m.Version()
	logger.InfoContext(ctx, "migrations applied", slog.Uint64("version", uint64(v)), slog.Bool("dirty", dirty))
	return err
}

// app is everything serve starts and stops.
type app struct {
	cfg        config.Config
	logger     *slog.Logger
	pool       *pgxpool.Pool
	server     *httpserver.Server
	dispatcher *outbox.Dispatcher
	// aiWorker runs Intelligence's translation jobs; nil when disabled.
	aiWorker   *intelligenceapp.Worker
	shutdownTP observability.ShutdownFunc
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	a, err := build(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer a.pool.Close()
	return a.run(ctx)
}

func build(ctx context.Context, cfg config.Config, logger *slog.Logger) (*app, error) {
	tp, shutdownTP, err := observability.NewTracerProvider(ctx, cfg.OTel, version())
	if err != nil {
		return nil, err
	}
	pool, err := db.OpenPool(ctx, cfg.DatabaseURL.Reveal(), "glossa-server")
	if err != nil {
		return nil, err
	}
	if err := db.VerifyAppRole(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	registry := observability.NewRegistry()
	events := outbox.NewRegistry() // bounded contexts Subscribe here
	dispatcher, err := newDispatcher(cfg, logger, tp, registry, pool, events)
	if err != nil {
		pool.Close()
		return nil, err
	}
	identity, identitySvc, err := newIdentity(cfg.Identity, logger, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	bounded, err := buildContexts(cfg, logger, pool, events, registry)
	if err != nil {
		pool.Close()
		return nil, err
	}
	server := httpserver.New(cfg.HTTP, httpserver.Deps{
		Logger:         logger,
		TracerProvider: tp,
		Registry:       registry,
		Readiness:      []httpserver.Check{{Name: "postgres", Probe: pool.Ping}},
		Routes:         apiRoutes(identity, &metaAPI{signIn: identitySvc, edgeURL: cfg.Release.EdgePublicURL}, bounded),
	})
	return &app{cfg: cfg, logger: logger, pool: pool, server: server, dispatcher: dispatcher, aiWorker: bounded.aiWorker, shutdownTP: shutdownTP}, nil
}

func newDispatcher(
	cfg config.Config, logger *slog.Logger, tp trace.TracerProvider,
	reg prometheus.Registerer, pool *pgxpool.Pool, events *outbox.Registry,
) (*outbox.Dispatcher, error) {
	store := outbox.NewPostgresStore(db.NewUnitOfWork(pool))
	return outbox.NewDispatcher(store, events, outbox.DispatcherConfig{
		BatchSize:      cfg.Outbox.BatchSize,
		MaxAttempts:    cfg.Outbox.MaxAttempts,
		PollInterval:   cfg.Outbox.PollInterval,
		Lease:          cfg.Outbox.Lease,
		HandlerTimeout: cfg.Outbox.HandlerTimeout,
	}, outbox.DispatcherOptions{Logger: logger, TracerProvider: tp, Registerer: reg})
}

// run serves until ctx is cancelled or a component fails, then shuts
// down in order: stop taking traffic and drain requests, stop the
// dispatcher (unstarted claims are released), flush traces.
func (a *app) run(ctx context.Context) error {
	ln, err := a.server.Listen()
	if err != nil {
		return err
	}
	a.logger.InfoContext(ctx, "listening", slog.String("addr", ln.Addr().String()))

	errc := make(chan error, 2)
	go func() { errc <- a.server.Serve(ln) }()
	dispatchCtx, stopDispatch := context.WithCancel(context.WithoutCancel(ctx))
	defer stopDispatch()
	dispatched := a.startDispatcher(dispatchCtx, errc)
	worked := a.startWorker(dispatchCtx)

	var runErr error
	select {
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	case runErr = <-errc:
		a.logger.Error("component failed; shutting down", slog.Any("error", runErr))
	}
	return errors.Join(runErr, a.shutdown(stopDispatch, dispatched, worked))
}

// startWorker runs Intelligence's job workers until ctx ends; a job in
// progress finishes first (bounded by its timeout).
func (a *app) startWorker(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if a.aiWorker == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		_ = a.aiWorker.Run(ctx)
	}()
	return done
}

func (a *app) startDispatcher(ctx context.Context, errc chan<- error) <-chan struct{} {
	done := make(chan struct{})
	if !a.cfg.Outbox.Enabled {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		if err := a.dispatcher.Run(ctx); err != nil {
			errc <- err
		}
	}()
	return done
}

func (a *app) shutdown(stopDispatch context.CancelFunc, dispatched, worked <-chan struct{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()
	var errs []error
	if err := a.server.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	stopDispatch()
	select {
	case <-dispatched:
	case <-ctx.Done():
		errs = append(errs, errors.New("outbox dispatcher did not stop before the shutdown timeout"))
	}
	select {
	case <-worked:
	case <-ctx.Done():
		errs = append(errs, errors.New("AI job workers did not stop before the shutdown timeout; their jobs are claimed again when the lease ends"))
	}
	if err := a.shutdownTP(ctx); err != nil {
		errs = append(errs, fmt.Errorf("flush traces: %w", err))
	}
	a.logger.Info("glossa-server stopped")
	return errors.Join(errs...)
}

func tenantAttrs(ctx context.Context) []slog.Attr {
	if id, ok := tenancy.FromContext(ctx); ok {
		return []slog.Attr{slog.String("tenant_id", id.String())}
	}
	return nil
}

// version reports the module version or VCS revision from build info.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 12 {
			return s.Value[:12]
		}
	}
	return "devel"
}
