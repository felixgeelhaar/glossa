package httpgin

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/felixgeelhaar/fortify/ratelimit"
	"github.com/gin-gonic/gin"

	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

// APIKeyResolver is the narrow port the API-key middleware needs.
// Wider than necessary for now — the only operation is
// FindByAPIKeyHash, mirrored from [project.Repository] — but kept
// separate so consumer-side handlers can be tested without a real
// project repo.
type APIKeyResolver interface {
	FindByAPIKeyHash(ctx context.Context, hash []byte) (project.Project, error)
}

// slogMiddleware emits one structured log line per request with
// method, path, status, latency, and client IP. Uses the bolt-backed
// slog.Logger constructed in main.
func slogMiddleware(l *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		l.Info("http",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("latency", time.Since(start)),
			slog.String("client_ip", c.ClientIP()),
		)
	}
}

// rateLimitMiddleware applies fortify's rate-limit gate per client
// IP. Used as a global throttle; per-tenant limits live on top.
func rateLimitMiddleware(rl ratelimit.RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.Allow(c.Request.Context(), c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}
		c.Next()
	}
}
