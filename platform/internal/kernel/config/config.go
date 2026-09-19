// Package config loads glossa-server's configuration from the
// environment (12-factor). Everything is validated once at startup and
// every problem is reported in a single error, so a misconfigured
// deployment fails fast with the full list instead of one field at a
// time.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LookupFunc reads one environment variable; os.LookupEnv satisfies it.
type LookupFunc func(key string) (string, bool)

// MigrateMode controls whether glossa-server applies migrations at boot.
type MigrateMode string

const (
	// MigrateOff never touches the schema (the default; migrations run
	// as a separate deploy step).
	MigrateOff MigrateMode = "off"
	// MigrateUp applies pending migrations, then serves.
	MigrateUp MigrateMode = "up"
	// MigrateOnly applies pending migrations and exits (k8s Job mode).
	MigrateOnly MigrateMode = "only"
)

// Secret holds a value that must never be logged, such as a DSN with a
// password. String redacts; Reveal returns the raw value.
type Secret struct{ value string }

// String implements fmt.Stringer with the password (if any) redacted.
func (s Secret) String() string {
	if s.value == "" {
		return ""
	}
	u, err := url.Parse(s.value)
	if err != nil || u.Scheme == "" {
		return "[redacted]"
	}
	return u.Redacted()
}

// Reveal returns the raw secret for the one place that needs it.
func (s Secret) Reveal() string { return s.value }

// IsZero reports whether the secret is unset.
func (s Secret) IsZero() bool { return s.value == "" }

// HTTP configures the public listener.
type HTTP struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxBodyBytes      int64
}

// OTel configures trace export. Standard OTEL_* variables beyond the
// endpoint (headers, protocol, sampler) are read by the SDK itself.
type OTel struct {
	Endpoint    string
	ServiceName string
}

// Enabled reports whether an OTLP exporter should be installed.
func (o OTel) Enabled() bool { return o.Endpoint != "" }

// Outbox configures the in-process outbox dispatcher.
type Outbox struct {
	Enabled        bool
	PollInterval   time.Duration
	BatchSize      int
	MaxAttempts    int
	Lease          time.Duration
	HandlerTimeout time.Duration
}

// Config is the complete, validated configuration.
type Config struct {
	// DatabaseURL is the application role's DSN (NOSUPERUSER,
	// NOBYPASSRLS). Pool settings go in the DSN (pool_max_conns=…).
	DatabaseURL Secret
	// MigrationDatabaseURL is the schema owner's DSN, used only for
	// migrations. Required when Migrate is up or only.
	MigrationDatabaseURL Secret
	Migrate              MigrateMode
	HTTP                 HTTP
	LogLevel             slog.Level
	ShutdownTimeout      time.Duration
	OTel                 OTel
	Outbox               Outbox
}

// String renders the configuration with secrets redacted.
func (c Config) String() string {
	return fmt.Sprintf(
		"database=%s migrate=%s http=%s log=%s shutdown=%s otel=%q outbox=%t",
		c.DatabaseURL, c.Migrate, c.HTTP.Addr, c.LogLevel, c.ShutdownTimeout,
		c.OTel.Endpoint, c.Outbox.Enabled,
	)
}

// Load reads and validates the configuration.
func Load(lookup LookupFunc) (Config, error) {
	r := reader{lookup: lookup}
	cfg := Config{
		DatabaseURL:          Secret{r.required("DATABASE_URL")},
		MigrationDatabaseURL: Secret{r.str("MIGRATION_DATABASE_URL", "")},
		Migrate:              r.migrateMode("GLOSSA_MIGRATE"),
		HTTP: HTTP{
			Addr:              r.str("GLOSSA_HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: r.duration("GLOSSA_HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       r.duration("GLOSSA_HTTP_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:      r.duration("GLOSSA_HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:       r.duration("GLOSSA_HTTP_IDLE_TIMEOUT", 120*time.Second),
			MaxBodyBytes:      int64(r.intRange("GLOSSA_HTTP_MAX_BODY_BYTES", 1<<20, 1, 1<<30)),
		},
		LogLevel:        r.logLevel("GLOSSA_LOG_LEVEL"),
		ShutdownTimeout: r.duration("GLOSSA_SHUTDOWN_TIMEOUT", 25*time.Second),
		OTel: OTel{
			Endpoint:    r.str("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			ServiceName: r.str("OTEL_SERVICE_NAME", "glossa-server"),
		},
		Outbox: Outbox{
			Enabled:        r.boolean("GLOSSA_OUTBOX_ENABLED", true),
			PollInterval:   r.duration("GLOSSA_OUTBOX_POLL_INTERVAL", time.Second),
			BatchSize:      r.intRange("GLOSSA_OUTBOX_BATCH_SIZE", 50, 1, 1000),
			MaxAttempts:    r.intRange("GLOSSA_OUTBOX_MAX_ATTEMPTS", 10, 1, 100),
			Lease:          r.duration("GLOSSA_OUTBOX_LEASE", time.Minute),
			HandlerTimeout: r.duration("GLOSSA_OUTBOX_HANDLER_TIMEOUT", 30*time.Second),
		},
	}
	cfg.validate(&r)
	if len(r.errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  %w", errors.Join(r.errs...))
	}
	return cfg, nil
}

// validate checks the cross-field rules.
func (c Config) validate(r *reader) {
	if c.Migrate != MigrateOff && c.MigrationDatabaseURL.IsZero() {
		r.fail("MIGRATION_DATABASE_URL", "required when GLOSSA_MIGRATE is up or only")
	}
	if c.Outbox.Lease <= c.Outbox.HandlerTimeout {
		r.fail("GLOSSA_OUTBOX_LEASE", "must be longer than GLOSSA_OUTBOX_HANDLER_TIMEOUT")
	}
}

// reader accumulates parse errors so Load can report all of them.
type reader struct {
	lookup LookupFunc
	errs   []error
}

func (r *reader) fail(key, format string, args ...any) {
	r.errs = append(r.errs, fmt.Errorf("%s: "+format, append([]any{key}, args...)...))
}

func (r *reader) str(key, def string) string {
	if v, ok := r.lookup(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func (r *reader) required(key string) string {
	v := r.str(key, "")
	if v == "" {
		r.fail(key, "required")
	}
	return v
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	raw := r.str(key, "")
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		r.fail(key, "invalid duration %q", raw)
		return def
	}
	if d <= 0 {
		r.fail(key, "must be positive")
	}
	return d
}

func (r *reader) intRange(key string, def, lo, hi int) int {
	raw := r.str(key, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		r.fail(key, "must be between %d and %d", lo, hi)
		return def
	}
	return n
}

func (r *reader) boolean(key string, def bool) bool {
	raw := r.str(key, "")
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		r.fail(key, "invalid boolean %q", raw)
		return def
	}
	return b
}

func (r *reader) migrateMode(key string) MigrateMode {
	raw := strings.ToLower(r.str(key, string(MigrateOff)))
	switch m := MigrateMode(raw); m {
	case MigrateOff, MigrateUp, MigrateOnly:
		return m
	}
	r.fail(key, "must be one of off, up, only (got %q)", raw)
	return MigrateOff
}

func (r *reader) logLevel(key string) slog.Level {
	raw := strings.ToLower(r.str(key, "info"))
	levels := map[string]slog.Level{
		"debug": slog.LevelDebug, "info": slog.LevelInfo,
		"warn": slog.LevelWarn, "error": slog.LevelError,
	}
	if l, ok := levels[raw]; ok {
		return l
	}
	r.fail(key, "must be one of debug, info, warn, error (got %q)", raw)
	return slog.LevelInfo
}
