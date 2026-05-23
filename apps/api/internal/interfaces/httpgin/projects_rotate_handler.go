package httpgin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	projectapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/project"
)

// handleRotateAPIKey rotates the project's stored hash and returns
// the new raw key exactly once.
//
// POST /api/v1/projects/:slug/api-keys
func handleRotateAPIKey(uc *projectapp.RotateAPIKey) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authedProject(c)
		raw, err := uc.Execute(c.Request.Context(), p.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"apiKey": raw})
	}
}
