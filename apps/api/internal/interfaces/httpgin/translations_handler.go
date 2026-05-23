package httpgin

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	translationapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translation"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/audit"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/locale"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/user"
)

// handleListBundle returns the full (project, locale) message map.
// Used by the SDK at runtime, the CLI at build time, and the admin
// UI to populate the key list.
func handleListBundle(
	uc *translationapp.ListBundle,
	projects project.Repository,
	locales locale.Repository,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := resolveProject(c, projects)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		code := c.Param("locale")
		all, err := locales.ListForProject(contextOf(c), p.ID)
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
		entries, err := uc.Execute(contextOf(c), p.ID, found.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
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
// Supports both auth flows. Translators (JWT path) are scoped to
// the locales listed on their user record; an out-of-scope edit
// returns 403. The change fans out via SSE and lands in the audit
// log under the resolved tenant.
func handlePatchTranslation(
	uc *translationapp.UpdateTranslation,
	projects project.Repository,
	locales locale.Repository,
	keys keysFinder,
	pub translationapp.Publisher,
	audits audit.Repository,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := resolveProject(c, projects)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		localeCode := c.Param("locale")
		keyName := c.Param("key")

		// Translator locale scoping. API-key callers carry no role
		// so they're treated as service-level and skip this check.
		if role, ok := c.Get(ctxKeyUserRole); ok && role == string(user.RoleTranslator) {
			if !sliceContains(authedUserLocales(c), localeCode) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "translator not scoped to this locale",
				})
				return
			}
		}

		var req patchTranslationReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		allLocales, err := locales.ListForProject(contextOf(c), p.ID)
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

		k, err := keys.FindByName(contextOf(c), p.ID, keyName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "key not found for this project"})
			return
		}

		// Audit actor: JWT path supplies it directly; API-key path
		// optionally reads it from the body; otherwise zero (CLI /
		// system change).
		actor := uuid.Nil
		if v, ok := c.Get(ctxKeyUserID); ok {
			actor, _ = v.(uuid.UUID)
		} else if req.UpdatedBy != "" {
			actor, _ = uuid.Parse(req.UpdatedBy)
		}

		out, err := uc.Execute(contextOf(c), translationapp.UpdateInput{
			KeyID:     k,
			LocaleID:  l.ID,
			Value:     req.Value,
			Status:    req.Status,
			UpdatedBy: actor,
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}

		tenantID, _ := c.Get(ctxKeyTenantID)
		_ = audits.Append(contextOf(c), audit.Entry{
			TenantID:      tenantID.(uuid.UUID),
			TranslationID: out.ID,
			BeforeValue:   "",
			AfterValue:    out.Value,
			ChangedBy:     actor,
		})
		pub.Publish(p.ID, tenantID.(uuid.UUID), translationapp.Event{
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

func sliceContains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// keysFinder resolves a (project, key name) pair to a key UUID.
// Implementation in main.go bridges to translationkey.Repository.
type keysFinder interface {
	FindByName(ctx context.Context, projectID uuid.UUID, name string) (uuid.UUID, error)
}
