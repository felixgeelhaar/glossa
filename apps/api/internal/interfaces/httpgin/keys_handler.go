package httpgin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	keyapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translationkey"
)

type scanKeysReq struct {
	Keys []keyScanRow `json:"keys" binding:"required"`
}

type keyScanRow struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// handleScanKeys is the batch UPSERT endpoint the CLI's `glossa scan`
// command targets. Per-row errors are returned alongside per-row
// successes so the CLI can map them back to source locations.
func handleScanKeys(uc *keyapp.UpsertKeys) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authedProject(c)
		var req scanKeysReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		inputs := make([]keyapp.UpsertInput, len(req.Keys))
		for i, r := range req.Keys {
			inputs[i] = keyapp.UpsertInput{Name: r.Name, Description: r.Description}
		}
		results, err := uc.Execute(c.Request.Context(), p.ID, inputs)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		out := make([]gin.H, 0, len(results))
		for _, r := range results {
			row := gin.H{"name": r.Input.Name}
			if r.Err != nil {
				row["error"] = r.Err.Error()
			} else {
				row["id"] = r.Key.ID.String()
				row["description"] = r.Key.Description
			}
			out = append(out, row)
		}
		c.JSON(http.StatusOK, gin.H{"results": out})
	}
}
