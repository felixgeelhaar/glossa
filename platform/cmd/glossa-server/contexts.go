package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/coverage"
	catalogapi "github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/httpapi"
	catalogpg "github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/postgres"
	"github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/projection"
	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	contextcatalog "github.com/felixgeelhaar/glossa/platform/internal/context/adapters/catalog"
	contextapi "github.com/felixgeelhaar/glossa/platform/internal/context/adapters/httpapi"
	"github.com/felixgeelhaar/glossa/platform/internal/context/adapters/imaging"
	contextmetrics "github.com/felixgeelhaar/glossa/platform/internal/context/adapters/metrics"
	contextpg "github.com/felixgeelhaar/glossa/platform/internal/context/adapters/postgres"
	contextapp "github.com/felixgeelhaar/glossa/platform/internal/context/app"
	contextdomain "github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	integrationapi "github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/httpapi"
	integrationpg "github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/postgres"
	integrationsources "github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/sources"
	integrationapp "github.com/felixgeelhaar/glossa/platform/internal/integration/app"
	intelligenceapi "github.com/felixgeelhaar/glossa/platform/internal/intelligence/adapters/httpapi"
	intelligencemetrics "github.com/felixgeelhaar/glossa/platform/internal/intelligence/adapters/metrics"
	intelligencepg "github.com/felixgeelhaar/glossa/platform/internal/intelligence/adapters/postgres"
	intelligenceproviders "github.com/felixgeelhaar/glossa/platform/internal/intelligence/adapters/providers"
	intelligencesources "github.com/felixgeelhaar/glossa/platform/internal/intelligence/adapters/sources"
	intelligenceapp "github.com/felixgeelhaar/glossa/platform/internal/intelligence/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/config"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/httpserver"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/objectstore"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/objectstore/configured"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/ratelimit"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/scheduler"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/sealing"
	knowledgeapi "github.com/felixgeelhaar/glossa/platform/internal/knowledge/adapters/httpapi"
	knowledgepg "github.com/felixgeelhaar/glossa/platform/internal/knowledge/adapters/postgres"
	knowledgesources "github.com/felixgeelhaar/glossa/platform/internal/knowledge/adapters/sources"
	knowledgeapp "github.com/felixgeelhaar/glossa/platform/internal/knowledge/app"
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
// translation coverage through coverage, Release and Knowledge read
// both through their sources adapters, and Intelligence reads (and
// writes translations) through its own.
type contexts struct {
	catalogAPI      *catalogapi.API
	localizationAPI *localizationapi.API
	releaseAPI      *releaseapi.API
	knowledgeAPI    *knowledgeapi.API
	intelligenceAPI *intelligenceapi.API
	// previewAPI is the stateless message preview (no database).
	previewAPI *previewapi.API
	// aiWorker runs Intelligence's jobs; nil when disabled.
	aiWorker *intelligenceapp.Worker
	// integrationAPI serves import and export jobs; integrationWorker
	// runs them (nil when disabled).
	integrationAPI    *integrationapi.API
	integrationWorker *integrationapp.Worker
	// usageContext is the Context context (RFC 0004): where messages
	// appear. contextAPI serves its uploads and reads; Intelligence's
	// message_context reads it through its own port.
	usageContext *contextapp.Service
	contextAPI   *contextapi.API
	// keyIndexes is Release's key index task: it rewrites the index
	// objects of keys written before their current format (migration
	// 0015 gave existing keys a scope).
	keyIndexes func(context.Context) (int, error)
	// purgeJobs are the daily retention jobs (RFC 0004 §2.3): Context's
	// builds, captures and images, and Catalog's proposal sweep. The
	// scheduler leases each so only one replica runs it.
	purgeJobs []scheduler.Job
	// branchPublisher publishes branch environments whose debounced
	// request is due (RFC 0004 §4). A publish is keyed by its request,
	// so every replica may run it; nil when
	// GLOSSA_BRANCH_PUBLISHER_ENABLED is off.
	branchPublisher *releaseapp.Publisher
}

// newPurgeJobs builds the daily retention jobs over the two contexts that
// have something to purge. Each logs what it did; a failure is the
// scheduler's to count and retry at the next interval.
func newPurgeJobs(usages *contextapp.Service, catalog *catalogapp.Service, logger *slog.Logger) []scheduler.Job {
	return []scheduler.Job{
		{Name: "context.purge", Run: func(ctx context.Context) error {
			purged, err := usages.Purge(ctx)
			for _, p := range purged {
				logger.InfoContext(ctx, "context: retention purged a project",
					slog.String("tenant_id", p.Tenant.String()), slog.String("project_id", p.Project.String()),
					slog.Int("builds", len(p.Builds)), slog.Int("captures", p.Captures),
					slog.Int("images", p.ImagesDeleted))
			}
			return err
		}},
		{Name: "catalog.proposals", Run: func(ctx context.Context) error {
			n, err := catalog.SweepAllProposals(ctx)
			if n > 0 {
				logger.InfoContext(ctx, "catalog: proposals of closed branches obsoleted", slog.Int("messages", n))
			}
			return err
		}},
	}
}

// Context's upload limits (RFC 0004 §10): a usages document is read
// within contextUploadTimeout, and a tenant uploads at most 10 a minute
// in bursts of up to 60 (a CI run uploads a few per application).
const contextUploadTimeout = 2 * time.Minute

// captureUploadTimeout bounds reading a capture upload: up to
// contextdomain.MaxCaptureUploadBytes (200 MB) of images from CI.
const captureUploadTimeout = 10 * time.Minute

func contextUploadLimit() ratelimit.Config {
	return ratelimit.Config{Rate: 10, Interval: time.Minute, Burst: 60}
}

// contextDeps are what the contexts need beyond the database.
type contextDeps struct {
	objects objectstore.StreamStore
	signer  *releasedomain.Signer
	logger  *slog.Logger
	// sealKey seals tenants' provider keys (derived from the auth
	// secret).
	sealKey    []byte
	registerer prometheus.Registerer
	// tracer traces CI uploads end to end (RFC 0004 §11).
	tracer      trace.TracerProvider
	ai          config.Intelligence
	integration config.Integration
	purge       config.Purge
	branches    config.Branches
	context     config.Context
}

// buildContexts opens object storage and the signer, then the contexts.
func buildContexts(
	cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool, events *outbox.Registry,
	reg prometheus.Registerer, tp trace.TracerProvider,
) (contexts, error) {
	objects, err := configured.Open(cfg.Storage)
	if err != nil {
		return contexts{}, err
	}
	signer, err := newSigner(cfg, logger)
	if err != nil {
		return contexts{}, err
	}
	sealKey, err := deriveKey(cfg.Identity.AuthKey(), "secret-seal")
	if err != nil {
		return contexts{}, err
	}
	return newContexts(pool, events, contextDeps{
		objects: objects, signer: signer, logger: logger, sealKey: sealKey, registerer: reg, ai: cfg.Intelligence,
		integration: cfg.Integration, purge: cfg.Purge, branches: cfg.Branches, context: cfg.Context, tracer: tp,
	})
}

// newContexts builds Catalog, Localization and Release and subscribes
// them to each other's events.
func newContexts(pool *pgxpool.Pool, events *outbox.Registry, deps contextDeps) (contexts, error) {
	uow := db.NewUnitOfWork(pool)
	catalog := catalogapp.New(catalogpg.NewTransactor(uow), catalogapp.WithScanner(catalogpg.NewScanner(uow)))
	localization := localizationapp.New(localizationpg.NewTransactor(uow), catalogport.New(catalog))
	translationPort := coverage.New(localization)
	catalog.SetCoverage(translationPort)
	catalog.SetImpact(translationPort)
	catalog.SetProjection(projection.New(localization))
	if err := localization.Subscribe(events); err != nil {
		return contexts{}, err
	}
	knowledge := knowledgeapp.New(knowledgepg.NewTransactor(uow), knowledgesources.NewTranslations(localization, catalog),
		knowledgesources.NewProjects(catalog), knowledgeapp.WithLogger(deps.logger))
	if err := knowledge.Subscribe(events); err != nil {
		return contexts{}, err
	}
	release := releaseapp.New(releasepg.NewTransactor(uow), sources.New(catalog, localization), deps.objects, deps.signer,
		releaseapp.WithLogger(deps.logger))
	if err := release.Subscribe(events); err != nil {
		return contexts{}, err
	}
	usageContext := contextapp.New(contextpg.NewTransactor(uow), contextcatalog.New(catalog),
		contextapp.WithSweeper(contextpg.NewSweeper(uow)), contextapp.WithLogger(deps.logger),
		contextapp.WithLimiter(ratelimit.New(contextUploadLimit())), contextapp.WithMetrics(contextmetrics.New(deps.registerer)),
		contextapp.WithImages(deps.objects, imaging.New("", imaging.DefaultDecodeBudget)),
		contextapp.WithDeleteBatch(deps.purge.BatchSize), contextapp.WithStorageQuota(deps.context.StorageQuotaBytes),
		contextapp.WithTracerProvider(deps.tracer))
	if err := usageContext.Subscribe(events); err != nil {
		return contexts{}, err
	}
	intelligence, err := newIntelligence(uow, catalog, localization, release, knowledge, usageContext, deps)
	if err != nil {
		return contexts{}, err
	}
	if err := intelligence.Subscribe(events); err != nil {
		return contexts{}, err
	}
	aiAPI, err := intelligenceapi.New(intelligence)
	if err != nil {
		return contexts{}, err
	}
	c := contexts{
		catalogAPI: catalogapi.New(catalog), localizationAPI: localizationapi.New(localization),
		releaseAPI: releaseapi.New(release), knowledgeAPI: knowledgeapi.New(knowledge), intelligenceAPI: aiAPI,
		previewAPI:   previewapi.New(previewapp.New(previewlimit.New(previewlimit.Default()))),
		usageContext: usageContext, contextAPI: contextapi.New(usageContext),
	}
	scanner := releasepg.NewScanner(uow)
	c.keyIndexes = func(ctx context.Context) (int, error) { return release.RewriteKeyIndexes(ctx, scanner) }
	c.purgeJobs = newPurgeJobs(usageContext, catalog, deps.logger)
	if deps.branches.PublisherEnabled {
		c.branchPublisher = releaseapp.NewPublisher(release, scanner, deps.branches.PublishInterval)
	}
	if deps.ai.WorkersEnabled {
		c.aiWorker = intelligenceapp.NewWorker(intelligence, intelligencepg.NewClaimer(uow), intelligenceapp.WorkerConfig{
			Workers: deps.ai.Workers, PollInterval: deps.ai.PollInterval, Lease: deps.ai.Lease, JobTimeout: deps.ai.JobTimeout,
		})
	}
	integration := integrationapp.New(integrationapp.Deps{
		Tx: integrationpg.NewTransactor(uow), Catalog: integrationsources.NewCatalog(catalog),
		Localization: integrationsources.NewLocalization(localization, catalog), Knowledge: integrationsources.NewKnowledge(knowledge),
		Objects: deps.objects, Logger: deps.logger,
		Config: integrationapp.Config{MaxUploadBytes: deps.integration.MaxUploadBytes, Retention: deps.integration.Retention},
	})
	if err := integration.Subscribe(events); err != nil {
		return contexts{}, err
	}
	c.integrationAPI = integrationapi.New(integration)
	if deps.integration.WorkersEnabled {
		c.integrationWorker = integrationapp.NewWorker(integration, integrationpg.NewClaimer(uow), integrationapp.WorkerConfig{
			Workers: deps.integration.Workers, PollInterval: deps.integration.PollInterval,
			Lease: deps.integration.Lease, JobTimeout: deps.integration.JobTimeout,
		})
	}
	return c, nil
}

// largeBodies lets import uploads and export downloads stream files
// larger and longer than the API's default body limit and timeouts,
// usage uploads reach the 20 MB of a usages document, and capture
// uploads the 200 MB of a manifest with its images.
func largeBodies(cfg config.Integration) func(*http.Request) (httpserver.BodyPolicy, bool) {
	return func(r *http.Request) (httpserver.BodyPolicy, bool) {
		switch {
		case contextapi.UploadPath(r.Method, r.URL.Path):
			return httpserver.BodyPolicy{MaxBytes: contextdomain.MaxUploadBytes, Timeout: contextUploadTimeout}, true
		case contextapi.CaptureUploadPath(r.Method, r.URL.Path):
			return httpserver.BodyPolicy{MaxBytes: contextdomain.MaxCaptureUploadBytes, Timeout: captureUploadTimeout}, true
		case integrationapi.UploadPath(r.Method, r.URL.Path):
			// The service enforces GLOSSA_INTEGRATION_MAX_UPLOAD_BYTES
			// while it streams, with its own problem code.
			return httpserver.BodyPolicy{Timeout: cfg.UploadTimeout}, true
		case integrationapi.DownloadPath(r.Method, r.URL.Path):
			return httpserver.BodyPolicy{MaxBytes: 1, Timeout: cfg.UploadTimeout}, true
		}
		return httpserver.BodyPolicy{}, false
	}
}

// newIntelligence wires the Intelligence context: its Postgres store,
// the other contexts' services through its sources adapters, providers
// built from tenants' configuration over an SSRF-safe client, sealed
// keys and Prometheus metrics.
func newIntelligence(uow *db.UnitOfWork, catalog *catalogapp.Service, localization *localizationapp.Service,
	release *releaseapp.Service, knowledge *knowledgeapp.Service, usages *contextapp.Service, deps contextDeps,
) (*intelligenceapp.Service, error) {
	sealer, err := sealing.New(deps.sealKey)
	if err != nil {
		return nil, err
	}
	return intelligenceapp.NewService(intelligenceapp.Deps{
		Tx:           intelligencepg.NewTransactor(uow),
		Catalog:      intelligencesources.NewCatalog(catalog),
		Localization: intelligencesources.NewLocalization(localization, catalog),
		Environments: intelligencesources.NewEnvironments(release),
		Knowledge:    intelligencesources.NewKnowledge(knowledge),
		Usages:       intelligencesources.NewUsages(usages),
		Providers: intelligenceproviders.New(intelligenceproviders.Config{
			AllowPrivate: deps.ai.AllowPrivateEndpoints, Concurrency: deps.ai.ProviderConcurrency, Logger: deps.logger,
		}),
		Sealer:                sealer,
		Metrics:               intelligencemetrics.New(deps.registerer),
		Logger:                deps.logger,
		AllowPrivateEndpoints: deps.ai.AllowPrivateEndpoints,
	})
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
