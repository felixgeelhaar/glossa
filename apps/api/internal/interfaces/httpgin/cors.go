package httpgin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// corsMiddleware enables cross-origin access for SDK consumers
// (Brotwerk web, IRI, etc.) that fetch bundles + subscribe to SSE
// from a different origin.
//
// Auth is Bearer-only; no cookies cross the boundary, so credentials
// stay false and a permissive origin policy is safe. Configure
// allowed origins via GLOSSA_CORS_ORIGINS (comma-separated). Empty
// = "*" (any origin), which is fine for the API-key-protected
// surface but pin a list once you know which consumers you serve.
func corsMiddleware(allowed []string) gin.HandlerFunc {
	allowAny := len(allowed) == 0
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		set[strings.TrimSpace(o)] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}
		echo := ""
		switch {
		case allowAny:
			echo = "*"
		default:
			if _, ok := set[origin]; ok {
				echo = origin
			}
		}
		if echo != "" {
			c.Header("Access-Control-Allow-Origin", echo)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, If-None-Match")
			c.Header("Access-Control-Expose-Headers", "ETag")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
