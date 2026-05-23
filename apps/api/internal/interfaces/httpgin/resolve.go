package httpgin

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

// resolveProject returns the project context for the current
// request. It supports both auth flows:
//
//   - api-key auth (CLI / SDK / consumer): [apiKeyAuth] stashed the
//     full project on the gin context. Return it directly.
//   - JWT admin auth: only the tenant_id is on the context. Read
//     the URL `:slug` parameter, validate it through the domain VO,
//     and look up the project under that tenant. RLS ensures
//     cross-tenant projects are invisible even if a slug collides.
//
// Returns an error so the handler can choose between 404 (no such
// project) and 500 (DB outage); callers should distinguish via
// errors.Is on the repo's ErrNotFound.
func resolveProject(c *gin.Context, projects project.Repository) (project.Project, error) {
	if v, ok := c.Get(ctxKeyProject); ok {
		return v.(project.Project), nil
	}
	tenantRaw, ok := c.Get(ctxKeyTenantID)
	if !ok {
		return project.Project{}, errors.New("resolve: no tenant on context — wiring bug")
	}
	tenantID, ok := tenantRaw.(uuid.UUID)
	if !ok {
		return project.Project{}, errors.New("resolve: tenant id wrong type")
	}
	slug, err := project.NewSlug(c.Param("slug"))
	if err != nil {
		return project.Project{}, err
	}
	return projects.Find(contextOf(c), tenantID, slug)
}

// contextOf is a tiny extractor so call sites read like
// `projects.Find(contextOf(c), ...)` instead of
// `c.Request.Context()` peppered everywhere.
func contextOf(c *gin.Context) context.Context { return c.Request.Context() }
