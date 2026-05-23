// Package logging wires the bolt slog handler — Glossa's standard
// structured-logger setup, matching the Brotwerk + IRI pattern.
package logging

import (
	"log/slog"
	"os"

	"github.com/felixgeelhaar/bolt"
)

// New returns a slog.Logger backed by bolt. Level is parsed from the
// LOG_LEVEL env var; unknown values fall back to INFO.
func New() *slog.Logger {
	lvl := parseLevel(os.Getenv("LOG_LEVEL"))
	handler := bolt.NewSlogHandler(os.Stdout, &bolt.SlogHandlerOptions{Level: lvl})
	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "warn", "WARN":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
