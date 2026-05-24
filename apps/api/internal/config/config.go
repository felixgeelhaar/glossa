// Package config carries the runtime configuration read from the
// environment.
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

	// JWT signing key (HS256). Required; min 32 bytes. Generated
	// once per deployment — leak ⇒ rotate everywhere.
	JWTSigningKey string

	// Secrets key for at-rest encryption of provider credentials
	// (AES-GCM). Required as 64-char hex if any AI translation
	// provider is configured; otherwise optional.
	SecretsKeyHex string

	// Admin bootstrap. All four are optional; if BootstrapTenantSlug
	// is empty, the bootstrap step is skipped entirely.
	BootstrapTenantSlug    string
	BootstrapTenantName    string
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
}

// ErrMissingDatabaseURL is returned by [Load] when DATABASE_URL is
// not set.
var ErrMissingDatabaseURL = errors.New("config: DATABASE_URL is required")

// ErrMissingJWTSigningKey is returned by [Load] when JWT_SIGNING_KEY
// is not set or too short.
var ErrMissingJWTSigningKey = errors.New("config: JWT_SIGNING_KEY is required (min 32 bytes)")

// Load reads the runtime config from env.
func Load() (Config, error) {
	cfg := Config{
		Port:        envDefault("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		LogLevel:    envDefault("LOG_LEVEL", "info"),

		JWTSigningKey: os.Getenv("JWT_SIGNING_KEY"),

		SecretsKeyHex: os.Getenv("GLOSSA_SECRETS_KEY"),

		BootstrapTenantSlug:    os.Getenv("BOOTSTRAP_TENANT_SLUG"),
		BootstrapTenantName:    os.Getenv("BOOTSTRAP_TENANT_NAME"),
		BootstrapAdminEmail:    os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, ErrMissingDatabaseURL
	}
	if len(cfg.JWTSigningKey) < 32 {
		return cfg, ErrMissingJWTSigningKey
	}
	return cfg, nil
}

func envDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
