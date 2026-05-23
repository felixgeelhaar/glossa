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

	projectapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/project"
)

// Deps carries every dependency the router needs. Constructed once
// in cmd/api/main.go and passed in.
type Deps struct {
	Logger       *slog.Logger
	CreateProj   *projectapp.CreateProject
	GlobalLimit  ratelimit.RateLimiter // per-IP global throttle
	// ProjectRepo is the lookup used by the API-key middleware to
	// resolve a tenant from the inbound key. Wired separately from
	// the use case so handlers don't see the underlying repo type.
	ProjectRepo APIKeyResolver
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

	// Public REST API. The /api/v1 group will grow auth + tenant
	// scoping in follow-on commits (task-go-api scope §). For now we
	// expose the unauthenticated project-create endpoint that an
	// admin would otherwise drive through the admin UI — useful for
	// bootstrap testing before the admin lands.
	v1 := r.Group("/api/v1")
	{
		v1.POST("/projects", handleCreateProject(d.CreateProj))
	}

	return r
}

func healthz(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}
