// Package main wires Glossa's API binary. Loads config, builds the
// logger, opens the DB pool, constructs the use cases, and starts
// gin.
//
// Kept thin on purpose — every business decision lives in
// internal/app or internal/domain.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/felixgeelhaar/fortify/ratelimit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	projectapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/project"
	translationapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translation"
	keyapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translationkey"
	"github.com/felixgeelhaar/glossa/apps/api/internal/config"
	"github.com/felixgeelhaar/glossa/apps/api/internal/db"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/translationkey"
	"github.com/felixgeelhaar/glossa/apps/api/internal/infra/sqlcadapter"
	"github.com/felixgeelhaar/glossa/apps/api/internal/interfaces/httpgin"
	"github.com/felixgeelhaar/glossa/apps/api/internal/logging"
)

func main() {
	log := logging.New()
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("config load failed", slog.Any("err", err))
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Error("db pool open failed", slog.Any("err", err))
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		log.Error("db ping failed", slog.Any("err", err))
		os.Exit(1)
	}

	queries := db.New(pool)

	// Repos.
	projectRepo := sqlcadapter.NewProjectRepo(queries)
	localeRepo := sqlcadapter.NewLocaleRepo(queries)
	keyRepo := sqlcadapter.NewKeyRepo(queries)
	translationRepo := sqlcadapter.NewTranslationRepo(queries)

	// Use cases.
	createProj := projectapp.NewCreateProject(projectRepo)
	rotateKey := projectapp.NewRotateAPIKey(projectRepo)
	upsertKeys := keyapp.NewUpsertKeys(keyRepo)
	updateTr := translationapp.NewUpdateTranslation(translationRepo)
	listBundle := translationapp.NewListBundle(translationRepo)

	// In-process SSE hub. Single instance shared between the
	// PATCH translation handler (publisher) and the /sse handler
	// (subscriber side).
	hub := translationapp.NewHub()

	// Per-IP rate limit: 60 requests per minute. Bolt-backed slog +
	// fortify ratelimit mirror Brotwerk's setup.
	limiter := ratelimit.New(ratelimit.Config{
		Rate:     60,
		Burst:    60,
		Interval: time.Minute,
	})

	router := httpgin.New(httpgin.Deps{
		Logger:      log,
		GlobalLimit: limiter,
		Pool:        pool,

		CreateProj: createProj,
		RotateKey:  rotateKey,
		UpsertKeys: upsertKeys,
		UpdateTr:   updateTr,
		ListBundle: listBundle,
		Hub:        hub,

		ProjectRepo: projectRepo,
		Locales:     localeRepo,
		Keys:        keysFinderAdapter{keyRepo},
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("glossa-api listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", slog.Any("err", err))
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM. Drain at most 10s before
	// hard-closing the listener.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown error", slog.Any("err", err))
	}
}

// keysFinderAdapter bridges the existing translationkey.Repository
// (which returns full Key aggregates) to the narrow `keysFinder`
// port the HTTP translation-update handler needs (just the UUID).
// Lives in main.go because it's a wiring concern, not a domain or
// HTTP-layer abstraction.
type keysFinderAdapter struct {
	repo translationkey.Repository
}

func (a keysFinderAdapter) FindByName(ctx context.Context, projectID uuid.UUID, name string) (uuid.UUID, error) {
	n, err := translationkey.NewName(name)
	if err != nil {
		return uuid.Nil, err
	}
	k, err := a.repo.Find(ctx, projectID, n)
	if err != nil {
		return uuid.Nil, err
	}
	return k.ID, nil
}
