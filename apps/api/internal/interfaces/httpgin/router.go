// Package httpgin is the HTTP delivery layer. Each handler binds
// request shape → use case → response shape. Handlers depend on
// use cases (app/) only; never on the domain or infra directly.
//
// Three auth flows compose here:
//
//   - none           — health probes + JWT login.
//   - api-key Bearer — CLI / SDK / consumer apps. `apiKeyAuth`
//                      resolves the project + tenant from the
//                      hashed key.
//   - JWT Bearer     — admin SPA + translator UI. `jwtAuth` reads
//                      tenant + role + locales out of the token.
//
// Both authed flows are wrapped in `rlsTxMiddleware` so every DB
// query runs in a tx with `SET LOCAL app.current_tenant`.
package httpgin

import (
	"log/slog"

	"github.com/felixgeelhaar/fortify/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	authapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/auth"
	projectapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/project"
	translationapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translation"
	keyapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translationkey"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/audit"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/locale"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/tenant"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/translation"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/user"
)

// Deps carries every dependency the router needs. Constructed once
// in cmd/api/main.go and passed in.
type Deps struct {
	Logger      *slog.Logger
	GlobalLimit ratelimit.RateLimiter

	// LoginLimit is a tighter per-IP limiter applied only to
	// /api/v1/auth/login — defends bcrypt against brute force.
	// Recommended: 5 req/min, burst 10.
	LoginLimit ratelimit.RateLimiter

	// Pool drives rlsTxMiddleware (BEGIN; SET LOCAL …) for every
	// authed request.
	Pool *pgxpool.Pool

	// Use cases.
	CreateProj *projectapp.CreateProject
	RotateKey  *projectapp.RotateAPIKey
	UpsertKeys *keyapp.UpsertKeys
	UpdateTr   *translationapp.UpdateTranslation
	ListBundle *translationapp.ListBundle
	Login      *authapp.Login

	// Hub fans translation.updated events from PATCH handlers to
	// SSE subscribers. Single instance per process; swap for a
	// Redis-backed Publisher when we go multi-replica.
	Hub      *translationapp.Hub
	JWTIssuer authapp.TokenIssuer

	// Repos.
	ProjectRepo  project.Repository
	Tenants      tenant.Repository
	Users        user.Repository
	Locales      locale.Repository
	Keys         keysFinder
	Audits       audit.Repository
	Translations translation.Repository
}

// New builds the gin engine + mounts every route.
func New(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(slogMiddleware(d.Logger))
	if d.GlobalLimit != nil {
		r.Use(rateLimitMiddleware(d.GlobalLimit))
	}

	// Probes.
	r.GET("/healthz", healthz)
	r.GET("/readyz", healthz)

	v1 := r.Group("/api/v1")

	// ── Unauthenticated ──────────────────────────────────────────
	loginHandlers := []gin.HandlerFunc{}
	if d.LoginLimit != nil {
		loginHandlers = append(loginHandlers, rateLimitMiddleware(d.LoginLimit))
	}
	loginHandlers = append(loginHandlers, handleLogin(d.Login, d.Tenants))
	v1.POST("/auth/login", loginHandlers...)

	// ── API-key authed (consumer / CLI / SDK) ────────────────────
	authed := v1.Group("/projects/:slug")
	authed.Use(apiKeyAuth(d.ProjectRepo))
	authed.Use(rlsTxMiddleware(d.Pool))
	{
		authed.GET("/locales", handleListLocales(d.Locales))
		authed.POST("/locales", handleCreateLocale(d.Locales))
		authed.POST("/keys:scan", handleScanKeys(d.UpsertKeys))
		authed.GET("/locales/:locale/messages", handleListBundle(d.ListBundle, d.ProjectRepo, d.Locales))
		authed.PATCH("/locales/:locale/keys/:key",
			handlePatchTranslation(d.UpdateTr, d.Translations, d.ProjectRepo, d.Locales, d.Keys, d.Hub, d.Audits))
		authed.POST("/api-keys", handleRotateAPIKey(d.RotateKey))
		authed.GET("/sse", handleSSE(d.Hub, 0))
	}

	// ── JWT authed (admin SPA / translator UI) ───────────────────
	admin := v1.Group("/admin")
	admin.Use(jwtAuth(d.JWTIssuer))
	admin.Use(rlsTxMiddleware(d.Pool))
	{
		// Identity check used by the SPA on boot.
		admin.GET("/me", handleMe())

		// Project list + create. Create stays admin-only.
		admin.GET("/projects", handleListProjects(d.ProjectRepo))
		adminProjectsCreate := admin.Group("")
		adminProjectsCreate.Use(requireAdmin())
		adminProjectsCreate.POST("/projects", handleCreateProject(d.CreateProj))

		// Per-project admin/translator routes. Translators can hit
		// listBundle + PATCH (PATCH enforces per-locale scoping
		// internally); admin-only ops are nested under requireAdmin.
		proj := admin.Group("/projects/:slug")
		{
			proj.GET("/locales", handleListLocales(d.Locales))
			proj.GET("/locales/:locale/messages", handleListBundle(d.ListBundle, d.ProjectRepo, d.Locales))
			proj.PATCH("/locales/:locale/keys/:key",
				handlePatchTranslation(d.UpdateTr, d.Translations, d.ProjectRepo, d.Locales, d.Keys, d.Hub, d.Audits))
			proj.GET("/sse", handleSSE(d.Hub, 0))

			adminOnly := proj.Group("")
			adminOnly.Use(requireAdmin())
			adminOnly.POST("/locales", handleCreateLocale(d.Locales))
			adminOnly.PATCH("/locales/:id", handleSetLocaleEnabled(d.Locales))
			adminOnly.DELETE("/locales/:id", handleDeleteLocale(d.Locales))
			adminOnly.POST("/keys:scan", handleScanKeys(d.UpsertKeys))
			adminOnly.POST("/locales/:locale/bulk",
				handleBulkImport(d.ProjectRepo, d.Locales, d.UpsertKeys, d.Keys, d.UpdateTr, d.Translations, d.Hub, d.Audits))
			adminOnly.GET("/diff", handleBundleDiff(d.ProjectRepo, d.Locales, d.ListBundle))
			adminOnly.POST("/api-keys", handleRotateAPIKey(d.RotateKey))
		}

		// Tenant-level admin endpoints.
		tenantAdmin := admin.Group("")
		tenantAdmin.Use(requireAdmin())
		tenantAdmin.GET("/users", handleListUsers(d.Users))
		tenantAdmin.POST("/users", handleCreateUser(d.Users))
		tenantAdmin.PATCH("/users/:id/locales", handleUpdateUserLocales(d.Users))
		tenantAdmin.DELETE("/users/:id", handleDeleteUser(d.Users))
		tenantAdmin.GET("/audit", handleListAudit(d.Audits))
	}

	return r
}

func healthz(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}
