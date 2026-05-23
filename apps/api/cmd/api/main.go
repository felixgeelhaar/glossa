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

	projectapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/project"
	"github.com/felixgeelhaar/glossa/apps/api/internal/interfaces/httpgin"
	"github.com/felixgeelhaar/glossa/apps/api/internal/logging"
)

func main() {
	log := logging.New()
	slog.SetDefault(log)

	port := envDefault("PORT", "8080")

	// Use cases. The sqlc-backed project repo lands when we wire
	// the DB pool — for now a nil repo keeps cmd/api buildable so
	// the rest of the wiring is exercisable end-to-end during early
	// development.
	createProj := projectapp.NewCreateProject(nil)

	// Per-IP rate limit: 60 requests per minute. Bolt-backed slog +
	// fortify ratelimit mirror Brotwerk's setup.
	limiter := ratelimit.New(ratelimit.Config{
		Rate:     60,
		Burst:    60,
		Interval: time.Minute,
	})

	router := httpgin.New(httpgin.Deps{
		Logger:      log,
		CreateProj:  createProj,
		GlobalLimit: limiter,
	})

	srv := &http.Server{
		Addr:              ":" + port,
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

func envDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
