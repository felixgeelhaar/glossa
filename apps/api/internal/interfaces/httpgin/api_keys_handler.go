package httpgin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apikeyapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/apikey"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/apikey"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
)

type apiKeyRow struct {
	ID         string     `json:"id"`
	Scope      string     `json:"scope"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

func mapAPIKeyRow(k apikey.Key) apiKeyRow {
	row := apiKeyRow{
		ID:        k.ID.String(),
		Scope:     string(k.Scope),
		Label:     k.Label,
		CreatedAt: k.CreatedAt,
	}
	if !k.LastUsedAt.IsZero() {
		t := k.LastUsedAt
		row.LastUsedAt = &t
	}
	if !k.RevokedAt.IsZero() {
		t := k.RevokedAt
		row.RevokedAt = &t
	}
	return row
}

func handleListAPIKeys(projects project.Repository, keys apikey.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := resolveProject(c, projects)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		rows, err := keys.List(contextOf(c), p.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out := make([]apiKeyRow, 0, len(rows))
		for _, k := range rows {
			out = append(out, mapAPIKeyRow(k))
		}
		c.JSON(http.StatusOK, gin.H{"keys": out})
	}
}

type issueAPIKeyReq struct {
	Scope string `json:"scope" binding:"required"`
	Label string `json:"label" binding:"required"`
}

func handleIssueAPIKey(projects project.Repository, issue *apikeyapp.IssueAPIKey) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := resolveProject(c, projects)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		var req issueAPIKeyReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		scope, err := apikey.ParseScope(req.Scope)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		out, err := issue.Execute(contextOf(c), apikeyapp.IssueInput{
			ProjectID: p.ID,
			Scope:     scope,
			Label:     req.Label,
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"key":    mapAPIKeyRow(out.Key),
			"apiKey": out.Raw,
		})
	}
}

func handleRevokeAPIKey(revoke *apikeyapp.RevokeAPIKey) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		if err := revoke.Execute(contextOf(c), id); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}
