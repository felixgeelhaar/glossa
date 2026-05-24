package httpgin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	projectapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/project"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

// handleRotateAPIKey rotates the project's stored hash and returns
// the new raw key exactly once.
//
// Mounted under both the API-key path (/projects/:slug/api-keys)
// and the JWT admin path (/admin/projects/:slug/api-keys);
// resolveProject handles whichever auth flow planted the context.
func handleRotateAPIKey(projects project.Repository, uc *projectapp.RotateAPIKey) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := resolveProject(c, projects)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		raw, err := uc.Execute(c.Request.Context(), p.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"apiKey": raw})
	}
}
