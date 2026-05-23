// Package httpgin is the HTTP delivery layer. Each handler binds
// request shape → use case → response shape. Handlers depend on use
// cases (app/) only; never on the domain or infra directly.
//
// The router stitches together middleware (recovery, structured log,
// fortify rate limit) and the public + admin route groups.
package httpgin

import (
	"log/slog"

	"github.com/felixgeelhaar/fortify/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	projectapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/project"
	translationapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translation"
	keyapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translationkey"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/locale"
)

// Deps carries every dependency the router needs. Constructed once
// in cmd/api/main.go and passed in.
type Deps struct {
	Logger      *slog.Logger
	GlobalLimit ratelimit.RateLimiter // per-IP global throttle

	// Pool is the pgx pool used by rlsTxMiddleware to open a
	// per-request tx + SET LOCAL app.current_tenant. Required for
	// every authed route group; unauth bootstrap (POST /projects)
	// doesn't use it.
	Pool *pgxpool.Pool

	// Use cases.
	CreateProj *projectapp.CreateProject
	RotateKey  *projectapp.RotateAPIKey
	UpsertKeys *keyapp.UpsertKeys
	UpdateTr   *translationapp.UpdateTranslation
	ListBundle *translationapp.ListBundle

	// Repos used directly by handlers (locale lookups in the
	// translation flow, project lookup in the auth middleware).
	ProjectRepo APIKeyResolver
	Locales     locale.Repository
	Keys        keysFinder
}

// New builds the gin engine + mounts every route. gin's bundled
// recovery middleware handles panics; we add a structured logger and
// a global rate-limiter on top.
func New(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(slogMiddleware(d.Logger))
	if d.GlobalLimit != nil {
		r.Use(rateLimitMiddleware(d.GlobalLimit))
	}

	// Liveness + readiness — no auth, no rate limit beyond the global.
	r.GET("/healthz", healthz)
	r.GET("/readyz", healthz)

	// Public REST API.
	v1 := r.Group("/api/v1")
	{
		// Unauthenticated project bootstrap. Admin UI will drive
		// this via JWT once the admin auth flow lands; for now
		// any caller can create a project. Tightens with the
		// admin task.
		v1.POST("/projects", handleCreateProject(d.CreateProj))

		// API-key-authenticated routes. The middleware resolves
		// the project + tenant from the bearer token; :slug in
		// the URL is descriptive only (the route group can't
		// switch on it because auth is shared across them).
		authed := v1.Group("/projects/:slug")
		authed.Use(apiKeyAuth(d.ProjectRepo))
		authed.Use(rlsTxMiddleware(d.Pool))
		{
			authed.GET("/locales", handleListLocales(d.Locales))
			authed.POST("/locales", handleCreateLocale(d.Locales))
			authed.POST("/keys:scan", handleScanKeys(d.UpsertKeys))
			authed.GET("/locales/:locale/messages", handleListBundle(d.ListBundle, d.Locales))
			authed.PATCH("/locales/:locale/keys/:key", handlePatchTranslation(d.UpdateTr, d.Locales, d.Keys))
			authed.POST("/api-keys", handleRotateAPIKey(d.RotateKey))
		}
	}

	return r
}

func healthz(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}
