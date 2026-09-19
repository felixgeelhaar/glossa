package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/coverage"
	catalogapi "github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/httpapi"
	catalogpg "github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/postgres"
	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/config"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/objectstore"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/objectstore/configured"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	catalogport "github.com/felixgeelhaar/glossa/platform/internal/localization/adapters/catalog"
	localizationapi "github.com/felixgeelhaar/glossa/platform/internal/localization/adapters/httpapi"
	localizationpg "github.com/felixgeelhaar/glossa/platform/internal/localization/adapters/postgres"
	localizationapp "github.com/felixgeelhaar/glossa/platform/internal/localization/app"
	previewapi "github.com/felixgeelhaar/glossa/platform/internal/preview/adapters/httpapi"
	previewlimit "github.com/felixgeelhaar/glossa/platform/internal/preview/adapters/ratelimit"
	previewapp "github.com/felixgeelhaar/glossa/platform/internal/preview/app"
	releaseapi "github.com/felixgeelhaar/glossa/platform/internal/release/adapters/httpapi"
	releasepg "github.com/felixgeelhaar/glossa/platform/internal/release/adapters/postgres"
	"github.com/felixgeelhaar/glossa/platform/internal/release/adapters/sources"
	releaseapp "github.com/felixgeelhaar/glossa/platform/internal/release/app"
	"github.com/felixgeelhaar/glossa/platform/internal/release/delivery"
	releasedomain "github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// contexts are the bounded contexts besides Identity, wired to each
// other only through their application ports: Localization reads the
// catalog through catalogport, Catalog asks Localization for
// translation coverage through coverage, and Release reads both through
// sources.
type contexts struct {
	catalogAPI      *catalogapi.API
	localizationAPI *localizationapi.API
	releaseAPI      *releaseapi.API
	// previewAPI is the stateless message preview (no database).
	previewAPI *previewapi.API
}

// contextDeps are what the contexts need beyond the database.
type contextDeps struct {
	objects objectstore.Store
	signer  *releasedomain.Signer
	logger  *slog.Logger
}

// buildContexts opens object storage and the signer, then the contexts.
func buildContexts(cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool, events *outbox.Registry) (contexts, error) {
	objects, err := configured.Open(cfg.Storage)
	if err != nil {
		return contexts{}, err
	}
	signer, err := newSigner(cfg, logger)
	if err != nil {
		return contexts{}, err
	}
	return newContexts(pool, events, contextDeps{objects: objects, signer: signer, logger: logger})
}

// newContexts builds Catalog, Localization and Release and subscribes
// them to each other's events.
func newContexts(pool *pgxpool.Pool, events *outbox.Registry, deps contextDeps) (contexts, error) {
	uow := db.NewUnitOfWork(pool)
	catalog := catalogapp.New(catalogpg.NewTransactor(uow))
	localization := localizationapp.New(localizationpg.NewTransactor(uow), catalogport.New(catalog))
	catalog.SetCoverage(coverage.New(localization))
	if err := localization.Subscribe(events); err != nil {
		return contexts{}, err
	}
	release := releaseapp.New(releasepg.NewTransactor(uow), sources.New(catalog, localization), deps.objects, deps.signer,
		releaseapp.WithLogger(deps.logger))
	if err := release.Subscribe(events); err != nil {
		return contexts{}, err
	}
	return contexts{
		catalogAPI: catalogapi.New(catalog), localizationAPI: localizationapi.New(localization),
		releaseAPI: releaseapi.New(release),
		previewAPI: previewapi.New(previewapp.New(previewlimit.New(previewlimit.Default()))),
	}, nil
}

// newSigner builds the manifest signer from GLOSSA_RELEASE_SIGNING_KEYS
// and _RETIRED_KEYS. Without configured keys it derives one from the
// auth secret, for development: rotating that secret then rotates the
// signing key, which breaks runtimes pinned to it.
func newSigner(cfg config.Config, logger *slog.Logger) (*releasedomain.Signer, error) {
	var active []releasedomain.SigningKey
	for _, kv := range config.KeyList(cfg.Release.SigningKeys.Reveal()) {
		k, err := releasedomain.ParseSigningKey(kv[0], kv[1])
		if err != nil {
			return nil, fmt.Errorf("GLOSSA_RELEASE_SIGNING_KEYS: %w", err)
		}
		active = append(active, k)
	}
	if len(active) == 0 {
		seed, err := deriveKey(cfg.Identity.AuthKey(), "release-signing")
		if err != nil {
			return nil, err
		}
		pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
		id := "derived-" + delivery.Digest(pub)[:12]
		k, err := releasedomain.ParseSigningKey(id, base64.StdEncoding.EncodeToString(seed))
		if err != nil {
			return nil, err
		}
		active = append(active, k)
		logger.Warn("GLOSSA_RELEASE_SIGNING_KEYS is unset: manifests are signed with a key derived from GLOSSA_AUTH_SECRET; configure a dedicated key before runtimes pin it",
			slog.String("key_id", id))
	}
	var retired []releasedomain.PublicKey
	for _, kv := range config.KeyList(cfg.Release.RetiredKeys) {
		k, err := releasedomain.ParsePublicKey(kv[0], kv[1])
		if err != nil {
			return nil, fmt.Errorf("GLOSSA_RELEASE_RETIRED_KEYS: %w", err)
		}
		retired = append(retired, k)
	}
	return releasedomain.NewSigner(active, retired)
}
