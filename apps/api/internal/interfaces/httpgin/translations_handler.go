package httpgin

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	translationapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translation"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/locale"
)

// handleListBundle returns the full (project, locale) message map.
// Used by the SDK at runtime and the CLI at build time.
func handleListBundle(uc *translationapp.ListBundle, locales locale.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authedProject(c)
		code := c.Param("locale")
		// Resolve the locale by its code (URL-friendly) to its DB
		// UUID; otherwise the SDK would need to know UUIDs.
		all, err := locales.ListForProject(c.Request.Context(), p.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var found locale.Locale
		for _, l := range all {
			if l.Code.String() == code {
				found = l
				break
			}
		}
		if found.ID == uuid.Nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "locale not found for this project"})
			return
		}
		entries, err := uc.Execute(c.Request.Context(), p.ID, found.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Bundle format the SDK expects: a flat key→value map plus a
		// parallel key→status map so the SDK can tell approved from
		// pending entries.
		messages := make(map[string]string, len(entries))
		statuses := make(map[string]string, len(entries))
		for _, e := range entries {
			messages[e.Key] = e.Value
			if e.Status != "" {
				statuses[e.Key] = string(e.Status)
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"project":  p.Slug.String(),
			"locale":   code,
			"messages": messages,
			"statuses": statuses,
		})
	}
}

type patchTranslationReq struct {
	Value     string `json:"value" binding:"required"`
	Status    string `json:"status"`
	UpdatedBy string `json:"updatedBy"`
}

// handlePatchTranslation is the translator-edit endpoint.
// PATCH /api/v1/projects/:slug/locales/:locale/keys/:key
//
// On a successful update the handler fans the change out to every
// SSE subscriber on this project via the supplied [translationapp.Publisher]
// so consumer apps see the new value within a couple of network
// hops, no redeploy.
func handlePatchTranslation(
	uc *translationapp.UpdateTranslation,
	locales locale.Repository,
	keys keysFinder,
	pub translationapp.Publisher,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authedProject(c)
		localeCode := c.Param("locale")
		keyName := c.Param("key")

		var req patchTranslationReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Resolve locale UUID.
		allLocales, err := locales.ListForProject(c.Request.Context(), p.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var l locale.Locale
		for _, candidate := range allLocales {
			if candidate.Code.String() == localeCode {
				l = candidate
				break
			}
		}
		if l.ID == uuid.Nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "locale not found"})
			return
		}

		// Resolve key UUID.
		k, err := keys.FindByName(c.Request.Context(), p.ID, keyName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "key not found for this project"})
			return
		}

		updatedBy, _ := uuid.Parse(req.UpdatedBy)
		out, err := uc.Execute(c.Request.Context(), translationapp.UpdateInput{
			KeyID:     k,
			LocaleID:  l.ID,
			Value:     req.Value,
			Status:    req.Status,
			UpdatedBy: updatedBy,
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}

		// Broadcast — the SSE handler picks this up via the same
		// hub instance and delivers to every connected client.
		// Publish AFTER a successful tx commit (handler is past
		// any error returns), so subscribers never see a value
		// that was rolled back.
		pub.Publish(p.ID, translationapp.Event{
			Type:    "translation.updated",
			Project: p.Slug.String(),
			Locale:  localeCode,
			Key:     keyName,
			Value:   out.Value,
			Status:  string(out.Status),
		})

		c.JSON(http.StatusOK, gin.H{
			"id":     out.ID.String(),
			"value":  out.Value,
			"status": string(out.Status),
		})
	}
}

// keysFinder is the narrow port the translation-update handler needs:
// resolve a (project, name) pair to a key UUID. The implementation
// in main.go bridges to the existing translationkey.Repository.
type keysFinder interface {
	FindByName(ctx context.Context, projectID uuid.UUID, name string) (uuid.UUID, error)
}
