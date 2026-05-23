package httpgin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/locale"
)

type createLocaleReq struct {
	Code  string `json:"code" binding:"required"`
	Label string `json:"label" binding:"required"`
}

func handleCreateLocale(repo locale.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authedProject(c)
		var req createLocaleReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		code, err := locale.NewCode(req.Code)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		label, err := locale.NewLabel(req.Label)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		l := locale.Locale{
			ID:        uuid.New(),
			ProjectID: p.ID,
			Code:      code,
			Label:     label,
			Enabled:   true,
		}
		if err := repo.Save(c.Request.Context(), l); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"id":      l.ID.String(),
			"code":    l.Code.String(),
			"label":   l.Label.String(),
			"enabled": l.Enabled,
		})
	}
}

func handleListLocales(repo locale.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authedProject(c)
		rows, err := repo.ListForProject(c.Request.Context(), p.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out := make([]gin.H, 0, len(rows))
		for _, l := range rows {
			out = append(out, gin.H{
				"id":      l.ID.String(),
				"code":    l.Code.String(),
				"label":   l.Label.String(),
				"enabled": l.Enabled,
			})
		}
		c.JSON(http.StatusOK, out)
	}
}
