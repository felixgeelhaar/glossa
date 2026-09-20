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
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/ghoidc"
	identitymetrics "github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/metrics"
	identitysources "github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/sources"
	identityapp "github.com/felixgeelhaar/glossa/platform/internal/identity/app"
	"github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/github"
	integrationapi "github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/httpapi"
	integrationmetrics "github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/metrics"
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
	// githubInbox drains the GitHub webhook inbox (RFC 0004 §6.2); nil
	// when this deployment configures no GitHub App, or when
	// GLOSSA_GITHUB_INBOX_ENABLED is off. The install flow and the
	// endpoint still work without the worker — deliveries pile up
	// instead of being processed.
	githubInbox *integrationapp.InboxWorker
	// githubChecks renders pull requests' Glossa checks and writes them
	// to GitHub (RFC 0004 §6.4); nil without a GitHub App, or when
	// GLOSSA_GITHUB_CHECKS_ENABLED is off. The webhook still queues the
	// checks without it — they wait rather than being lost.
	githubChecks *integrationapp.CheckWorker
	// ciAuth is what Identity needs to exchange a GitHub Actions ID
	// token for a CI token (RFC 0004 §6.3): one process-wide verifier
	// and Integration's Git connections. Zero when this deployment has
	// no GitHub App, and the exchange then answers
	// `github_not_configured`.
	ciAuth identityapp.GitHubOIDC
}

// newPurgeJobs builds the daily retention jobs over the two contexts that
// have something to purge. Each logs what it did; a failure is the
// scheduler's to count and retry at the next interval.
func newPurgeJobs(usages *contextapp.Service, catalog *catalogapp.Service, inbox *integrationapp.InboxWorker,
	checks *integrationapp.CheckWorker, logger *slog.Logger,
) []scheduler.Job {
	jobs := []scheduler.Job{
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
	// The webhook inbox's periodic half: delivery IDs are kept for the
	// replay window and then dropped, along with install intents nobody
	// finished (RFC 0004 §6.2). The worker beside it is a queue worker,
	// because a delivery is handled within seconds of arriving; this one
	// is leased, because one replica a day is enough.
	if inbox != nil {
		jobs = append(jobs, scheduler.Job{Name: "integration.github.sweep", Run: inbox.Sweep})
	}
	// The check's thirty-minute wait (RFC 0004 §6.4). It only makes the
	// checks past their deadline due again; the worker beside it decides
	// what they conclude, so a check is completed in one place. Leased,
	// because one replica asking is enough.
	if checks != nil {
		jobs = append(jobs, scheduler.Job{Name: "integration.github.check_timeout", Run: checks.Sweep})
	}
	return jobs
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
	github      config.GitHub
	// studioURL is where the pull request's sticky comment links to the
	// branch (GLOSSA_STUDIO_URL); edgeURL is where a branch
	// environment's manifest is served (GLOSSA_EDGE_PUBLIC_URL, empty
	// when the deployment does not announce its edge).
	studioURL string
	edgeURL   string
	// lookup reads the environment for the GitHub App's own
	// configuration (RFC 0004 §14.1: platform configuration, not tenant
	// data, in one Kubernetes Secret).
	lookup config.LookupFunc
}

// buildContexts opens object storage and the signer, then the contexts.
func buildContexts(
	cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool, events *outbox.Registry,
	reg prometheus.Registerer, tp trace.TracerProvider, lookup config.LookupFunc,
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
		github: cfg.GitHub, studioURL: cfg.Identity.StudioURL, edgeURL: cfg.Release.EdgePublicURL, lookup: lookup,
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
	catalog.SetLocales(translationPort)
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
	if deps.integration.WorkersEnabled {
		c.integrationWorker = integrationapp.NewWorker(integration, integrationpg.NewClaimer(uow), integrationapp.WorkerConfig{
			Workers: deps.integration.Workers, PollInterval: deps.integration.PollInterval,
			Lease: deps.integration.Lease, JobTimeout: deps.integration.JobTimeout,
		})
	}
	gh, err := newGitHub(uow, checkSources{
		catalog: catalog, localization: localization, knowledge: knowledge,
		usages: usageContext, release: release,
	}, deps)
	if err != nil {
		return contexts{}, err
	}
	c.integrationAPI = integrationapi.New(integration, gh)
	if gh != nil {
		if err := gh.SubscribeChecks(events); err != nil {
			return contexts{}, err
		}
		if c.ciAuth, err = newCIAuth(gh, deps); err != nil {
			return contexts{}, err
		}
	}
	if gh != nil && deps.github.InboxEnabled {
		c.githubInbox = integrationapp.NewInboxWorker(gh, integrationapp.InboxConfig{
			Workers: deps.github.InboxWorkers, PollInterval: deps.github.PollInterval,
			Timeout: deps.github.HandlerTimeout, Lease: deps.github.Lease,
			DepthInterval: deps.github.DepthInterval,
		})
	}
	if gh != nil && deps.github.ChecksEnabled {
		c.githubChecks = integrationapp.NewCheckWorker(gh, integrationapp.CheckConfig{
			Workers: deps.github.CheckWorkers, PollInterval: deps.github.CheckPollInterval,
			Timeout: deps.github.CheckTimeout, Lease: deps.github.CheckLease,
			DepthInterval: deps.github.CheckDepthInterval,
		})
	}
	c.purgeJobs = newPurgeJobs(usageContext, catalog, c.githubInbox, c.githubChecks, deps.logger)
	return c, nil
}

// checkSources are the application services the Glossa PR check reads.
type checkSources struct {
	catalog      *catalogapp.Service
	localization *localizationapp.Service
	knowledge    *knowledgeapp.Service
	usages       *contextapp.Service
	release      *releaseapp.Service
}

// newGitHub wires the GitHub integration (RFC 0004 §6) when the
// deployment configures a GitHub App: the App's ID, key, webhook secret
// and OAuth credentials come from the environment, as one Kubernetes
// Secret in the platform's namespace (§14.1).
//
// With none of it set it returns nil, nil: the endpoints then answer
// `github_not_configured`, no webhook is accepted, no worker runs, and
// nothing else in the server changes. A partial configuration is an
// error rather than a silent half-integration, because a deployment
// that meant to enable GitHub should hear about the missing half.
func newGitHub(uow *db.UnitOfWork, src checkSources, deps contextDeps) (*integrationapp.GitHubService, error) {
	cfg, enabled, err := github.LoadConfig(deps.lookup)
	if err != nil {
		return nil, err
	}
	if !enabled {
		deps.logger.Info("GLOSSA_GITHUB_APP_ID is unset: the GitHub integration is off; its endpoints answer github_not_configured")
		return nil, nil //nolint:nilnil // no App is a valid configuration
	}
	client, err := github.New(cfg, github.Options{
		Logger: deps.logger, OnCall: integrationmetrics.NewGitHub(deps.registerer).OnCall,
	})
	if err != nil {
		return nil, err
	}
	hooks, err := github.NewWebhooks(cfg.WebhookSecret)
	if err != nil {
		return nil, err
	}
	deps.logger.Info("the GitHub integration is on", slog.Any("github", cfg))
	return integrationapp.NewGitHubService(integrationapp.GitHubDeps{
		Tx:           integrationpg.NewGitHubTransactor(uow),
		Inbox:        integrationpg.NewInbox(uow),
		Repositories: integrationpg.NewRepositories(uow),
		GitHub:       client,
		Verifier:     hooks,
		Events:       hooks,
		Branches:     integrationsources.NewBranches(src.catalog),
		Checks:       integrationpg.NewChecks(uow),
		Sources: integrationsources.NewChecks(integrationsources.ChecksDeps{
			Catalog: src.catalog, Localization: src.localization, Knowledge: src.knowledge,
			Usages: src.usages, Release: src.release, EdgeURL: deps.edgeURL,
		}),
		StudioURL:      deps.studioURL,
		Metrics:        integrationmetrics.NewWebhooks(deps.registerer),
		CheckMetrics:   integrationmetrics.NewChecks(deps.registerer),
		TracerProvider: deps.tracer,
		Logger:         deps.logger,
	})
}

// newCIAuth builds the GitHub Actions OIDC exchange Identity serves
// (RFC 0004 §6.3): one verifier for the process, over auth-go's cached
// JWKS, and Integration's Git connections as the repository directory.
//
// It is only called when a GitHub App is configured, because a
// deployment without one has no Git connections to match a
// `repository_id` against: the exchange then stays off and answers
// `github_not_configured`, and CI uses a stored API token instead.
func newCIAuth(gh *integrationapp.GitHubService, deps contextDeps) (identityapp.GitHubOIDC, error) {
	cfg, _, err := github.LoadConfig(deps.lookup)
	if err != nil {
		return identityapp.GitHubOIDC{}, err
	}
	verifier, err := ghoidc.New(ghoidc.Config{
		Issuer: cfg.OIDCIssuer, Audience: cfg.OIDCAudience, MaxStale: cfg.OIDCMaxStale,
	})
	if err != nil {
		return identityapp.GitHubOIDC{}, err
	}
	deps.logger.Info("CI can authenticate with GitHub Actions OIDC",
		slog.String("issuer", verifier.Issuer()), slog.String("audience", verifier.Audience()))
	return identityapp.GitHubOIDC{
		Verifier:     verifier,
		Repositories: identitysources.NewGitRepositories(gh),
		Metrics:      identitymetrics.NewCI(deps.registerer),
	}, nil
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
