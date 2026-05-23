// Package config carries the runtime configuration read from the
// environment. Kept tiny on purpose — every value has an explicit
// default so a developer can `go run ./cmd/api` against a local
// Postgres without setting anything.
package config

import (
	"errors"
	"os"
)

// Config is the resolved environment shape.
type Config struct {
	Port        string
	DatabaseURL string
	LogLevel    string
}

// ErrMissingDatabaseURL is returned by [Load] when DATABASE_URL is
// not set. Defaults are friendly but we refuse to start without
// somewhere to write.
var ErrMissingDatabaseURL = errors.New("config: DATABASE_URL is required")

// Load reads the runtime config from env.
func Load() (Config, error) {
	cfg := Config{
		Port:        envDefault("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		LogLevel:    envDefault("LOG_LEVEL", "info"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, ErrMissingDatabaseURL
	}
	return cfg, nil
}

func envDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
