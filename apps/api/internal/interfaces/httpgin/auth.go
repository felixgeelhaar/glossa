package httpgin

import (
	"crypto/sha256"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

// Gin context keys for the API-key auth flow.
const (
	ctxKeyProject  = "glossa.project"
	ctxKeyTenantID = "glossa.tenant_id"
)

// apiKeyAuth resolves the inbound Bearer token to a Project (and its
// owning tenant) via SHA-256 lookup, then stores both on the gin
// context for downstream handlers.
//
// The middleware deliberately does NOT bind a DB connection here.
// Each handler that needs RLS-aware reads opens a short-lived
// `BEGIN; SET LOCAL app.current_tenant = '...'; ... COMMIT` block
// around its queries via [WithTenant]. Stashing the tenant in the
// context keeps the auth path independent from the DB-binding path
// so handlers without DB access stay free of pool overhead.
func apiKeyAuth(resolver APIKeyResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing or malformed Authorization header (expected: Bearer glossa_...)",
			})
			return
		}
		hash := sha256.Sum256([]byte(raw))
		p, err := resolver.FindByAPIKeyHash(c.Request.Context(), hash[:])
		if err != nil {
			// We treat every lookup failure as 401 to avoid leaking
			// "key valid but project deleted" vs "no such key" via
			// status-code probing.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}
		c.Set(ctxKeyProject, p)
		c.Set(ctxKeyTenantID, p.TenantID.String())
		c.Next()
	}
}

// bearerToken pulls the token portion out of an Authorization header.
// Returns ok=false on any malformed input.
func bearerToken(h string) (string, bool) {
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// authedProject returns the Project that the API-key middleware
// resolved earlier in this request. Panics if called outside a route
// that ran [apiKeyAuth] — that's a wiring bug, not a runtime
// condition we want to handle gracefully.
func authedProject(c *gin.Context) project.Project {
	v, ok := c.Get(ctxKeyProject)
	if !ok {
		panic("authedProject called without apiKeyAuth middleware in the chain")
	}
	return v.(project.Project)
}
