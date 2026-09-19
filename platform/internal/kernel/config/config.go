// Package config loads glossa-server's configuration from the
// environment (12-factor). Everything is validated once at startup and
// every problem is reported in a single error, so a misconfigured
// deployment fails fast with the full list instead of one field at a
// time.
package config

import (
	"encoding/base64"
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
	Identity             Identity
	Storage              Storage
	Release              Release
	Intelligence         Intelligence
	Integration          Integration
}

// Integration configures the import and export jobs (RFC 0003 §5–§6):
// the workers that run in glossa-server, uploads and retention.
type Integration struct {
	// WorkersEnabled runs import/export workers in this process.
	WorkersEnabled bool
	// Workers is the number of jobs this process runs at once.
	Workers      int
	PollInterval time.Duration
	// JobTimeout bounds one attempt of a job; Lease (longer) is how long
	// a claimed job is reserved before another worker may take it over.
	JobTimeout time.Duration
	Lease      time.Duration
	// MaxUploadBytes bounds an import's file.
	MaxUploadBytes int64
	// UploadTimeout bounds reading one upload's body (and writing one
	// download), instead of the HTTP read and write timeouts.
	UploadTimeout time.Duration
	// Retention is how long uploaded and exported files are kept.
	Retention time.Duration
}

// Intelligence configures the AI translation job workers that run in
// glossa-server and the providers they call (RFC 0003 §3).
type Intelligence struct {
	// WorkersEnabled runs job workers in this process.
	WorkersEnabled bool
	// Workers is the number of jobs this process runs at once.
	Workers      int
	PollInterval time.Duration
	// JobTimeout bounds one job, model calls and retries included; Lease
	// (longer) is how long a claimed job is reserved before another
	// worker may take it over.
	JobTimeout time.Duration
	Lease      time.Duration
	// AllowPrivateEndpoints lets tenants point providers at loopback and
	// private addresses (self-hosted models in the cluster).
	AllowPrivateEndpoints bool
	// ProviderConcurrency caps the calls in flight per configured
	// provider in this process.
	ProviderConcurrency int
}

// Identity configures authentication (the Identity context).
type Identity struct {
	// AuthSecret is the root key (base64, ≥ 32 bytes) from which the CSRF,
	// TOTP-sealing and passkey-state keys are derived. Rotating it signs
	// everyone's CSRF tokens anew, drops in-flight passkey ceremonies and
	// makes enrolled TOTP secrets unreadable (people re-enroll).
	AuthSecret Secret
	// StudioURL is Studio's origin; emailed links point into it.
	StudioURL string
	// SessionTTL is a session's lifetime.
	SessionTTL time.Duration
	Mail       Mail
	WebAuthn   WebAuthn
}

// AuthKey returns the decoded AuthSecret (validated by Load).
func (i Identity) AuthKey() []byte {
	k, _ := decodeKey(i.AuthSecret.Reveal())
	return k
}

// Mail configures outbound email.
type Mail struct {
	// Driver is "none" (the default: the deployment sends no email, so
	// magic links and password reset by email are unavailable), "smtp",
	// or "log" (development only: mail, links included, goes to the log).
	Driver string
	From   string
	// SMTPAddr is host:port of the submission server.
	SMTPAddr           string
	SMTPUsername       string
	SMTPPassword       Secret
	SMTPAllowPlaintext bool
}

// Enabled reports whether email is sent (or, with log, written out).
func (m Mail) Enabled() bool { return m.Driver != "none" }

// WebAuthn configures the passkey relying party. Passkeys are off while
// RPID is empty.
type WebAuthn struct {
	RPID    string
	RPName  string
	Origins []string
}

// Enabled reports whether passkeys are configured.
func (w WebAuthn) Enabled() bool { return w.RPID != "" }

const minAuthKeyLen = 32

func decodeKey(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("not base64")
}

// String renders the configuration with secrets redacted.
func (c Config) String() string {
	return fmt.Sprintf(
		"database=%s migrate=%s http=%s log=%s shutdown=%s otel=%q outbox=%t storage=%s ai_workers=%d",
		c.DatabaseURL, c.Migrate, c.HTTP.Addr, c.LogLevel, c.ShutdownTimeout,
		c.OTel.Endpoint, c.Outbox.Enabled, c.Storage.Driver, c.aiWorkers(),
	)
}

func (c Config) aiWorkers() int {
	if !c.Intelligence.WorkersEnabled {
		return 0
	}
	return c.Intelligence.Workers
}

// Load reads and validates the configuration.
func Load(lookup LookupFunc) (Config, error) {
	r := reader{lookup: lookup}
	cfg := Config{
		DatabaseURL:          Secret{r.required("DATABASE_URL")},
		MigrationDatabaseURL: Secret{r.str("MIGRATION_DATABASE_URL", "")},
		Migrate:              r.migrateMode("GLOSSA_MIGRATE"),
		HTTP:                 r.http(":8080"),
		LogLevel:             r.logLevel("GLOSSA_LOG_LEVEL"),
		ShutdownTimeout:      r.duration("GLOSSA_SHUTDOWN_TIMEOUT", 25*time.Second),
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
	cfg.Identity = r.identity()
	cfg.Storage = r.storage()
	cfg.Release = r.release()
	cfg.Intelligence = Intelligence{
		WorkersEnabled:        r.boolean("GLOSSA_AI_WORKERS_ENABLED", true),
		Workers:               r.intRange("GLOSSA_AI_WORKERS", 2, 1, 64),
		PollInterval:          r.duration("GLOSSA_AI_POLL_INTERVAL", time.Second),
		JobTimeout:            r.duration("GLOSSA_AI_JOB_TIMEOUT", 10*time.Minute),
		Lease:                 r.duration("GLOSSA_AI_JOB_LEASE", 15*time.Minute),
		AllowPrivateEndpoints: r.boolean("GLOSSA_AI_ALLOW_PRIVATE_ENDPOINTS", false),
		ProviderConcurrency:   r.intRange("GLOSSA_AI_PROVIDER_CONCURRENCY", 4, 1, 256),
	}
	cfg.Integration = Integration{
		WorkersEnabled: r.boolean("GLOSSA_INTEGRATION_WORKERS_ENABLED", true),
		Workers:        r.intRange("GLOSSA_INTEGRATION_WORKERS", 1, 1, 64),
		PollInterval:   r.duration("GLOSSA_INTEGRATION_POLL_INTERVAL", time.Second),
		JobTimeout:     r.duration("GLOSSA_INTEGRATION_JOB_TIMEOUT", 30*time.Minute),
		Lease:          r.duration("GLOSSA_INTEGRATION_JOB_LEASE", 35*time.Minute),
		MaxUploadBytes: int64(r.intRange("GLOSSA_INTEGRATION_MAX_UPLOAD_BYTES", 64<<20, 1, 2<<30)),
		UploadTimeout:  r.duration("GLOSSA_INTEGRATION_UPLOAD_TIMEOUT", 10*time.Minute),
		Retention:      r.duration("GLOSSA_INTEGRATION_RETENTION", 7*24*time.Hour),
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
	if c.Intelligence.Lease <= c.Intelligence.JobTimeout {
		r.fail("GLOSSA_AI_JOB_LEASE", "must be longer than GLOSSA_AI_JOB_TIMEOUT")
	}
	if c.Integration.Lease <= c.Integration.JobTimeout {
		r.fail("GLOSSA_INTEGRATION_JOB_LEASE", "must be longer than GLOSSA_INTEGRATION_JOB_TIMEOUT")
	}
}

func (r *reader) identity() Identity {
	id := Identity{
		AuthSecret: Secret{r.required("GLOSSA_AUTH_SECRET")},
		StudioURL:  r.absoluteURL("GLOSSA_STUDIO_URL", "http://localhost:5173"),
		SessionTTL: r.duration("GLOSSA_SESSION_TTL", 14*24*time.Hour),
		Mail: Mail{
			Driver:             r.str("GLOSSA_MAIL_DRIVER", "none"),
			From:               r.str("GLOSSA_MAIL_FROM", "Glossa <no-reply@localhost>"),
			SMTPAddr:           r.str("GLOSSA_SMTP_ADDR", ""),
			SMTPUsername:       r.str("GLOSSA_SMTP_USERNAME", ""),
			SMTPPassword:       Secret{r.str("GLOSSA_SMTP_PASSWORD", "")},
			SMTPAllowPlaintext: r.boolean("GLOSSA_SMTP_ALLOW_PLAINTEXT", false),
		},
		WebAuthn: WebAuthn{
			RPID:   r.str("GLOSSA_WEBAUTHN_RP_ID", ""),
			RPName: r.str("GLOSSA_WEBAUTHN_RP_NAME", "Glossa"),
		},
	}
	if s := id.AuthSecret.Reveal(); s != "" {
		if k, err := decodeKey(s); err != nil || len(k) < minAuthKeyLen {
			r.fail("GLOSSA_AUTH_SECRET", "must be base64 of at least %d random bytes (openssl rand -base64 32)", minAuthKeyLen)
		}
	}
	switch id.Mail.Driver {
	case "none", "log":
	case "smtp":
		if id.Mail.SMTPAddr == "" {
			r.fail("GLOSSA_SMTP_ADDR", "required when GLOSSA_MAIL_DRIVER is smtp")
		}
	default:
		r.fail("GLOSSA_MAIL_DRIVER", "must be none, smtp or log (got %q)", id.Mail.Driver)
	}
	if id.WebAuthn.Enabled() {
		for _, o := range strings.Split(r.str("GLOSSA_WEBAUTHN_ORIGINS", id.StudioURL), ",") {
			if o = strings.TrimSpace(o); o != "" {
				id.WebAuthn.Origins = append(id.WebAuthn.Origins, o)
			}
		}
	}
	return id
}

func (r *reader) absoluteURL(key, def string) string {
	raw := r.str(key, def)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		r.fail(key, "must be an absolute http(s) URL (got %q)", raw)
	}
	return strings.TrimRight(raw, "/")
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
