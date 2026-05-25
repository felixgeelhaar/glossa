package httpgin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/apierr/ginerr"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/locale"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
	"github.com/felixgeelhaar/glossa/apps/api/internal/errs"
)

type createLocaleReq struct {
	Code  string `json:"code" binding:"required"`
	Label string `json:"label" binding:"required"`
}

func handleCreateLocale(projects project.Repository, repo locale.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := resolveProject(c, projects)
		if err != nil {
			ginerr.Send(c, errs.ProjectNotFound)
			return
		}
		var req createLocaleReq
		if err := c.ShouldBindJSON(&req); err != nil {
			ginerr.Send(c, errs.BadRequestFromErr(err))
			return
		}
		code, err := locale.NewCode(req.Code)
		if err != nil {
			ginerr.Send(c, errs.UnprocessableFromErr(err))
			return
		}
		label, err := locale.NewLabel(req.Label)
		if err != nil {
			ginerr.Send(c, errs.UnprocessableFromErr(err))
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
			ginerr.Send(c, errs.InternalFromErr(err))
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

func handleListLocales(projects project.Repository, repo locale.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := resolveProject(c, projects)
		if err != nil {
			ginerr.Send(c, errs.ProjectNotFound)
			return
		}
		rows, err := repo.ListForProject(c.Request.Context(), p.ID)
		if err != nil {
			ginerr.Send(c, errs.InternalFromErr(err))
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
