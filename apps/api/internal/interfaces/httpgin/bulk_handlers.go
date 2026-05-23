package httpgin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	keyapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translationkey"
	translationapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/translation"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/audit"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/locale"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/translation"
)

// handleBulkImport — POST /api/v1/admin/projects/:slug/locales/:locale/bulk
//
// Body: { "messages": { "key": "value", ... }, "status": "approved" }
// Upserts every (key, locale) pair atomically, creating missing
// keys on the fly. Used by the admin UI's import flow and by
// translators paste-importing a fully edited bundle.
func handleBulkImport(
	projects project.Repository,
	locales locale.Repository,
	keysUC *keyapp.UpsertKeys,
	keysFind keysFinder,
	uc *translationapp.UpdateTranslation,
	pub translationapp.Publisher,
	audits audit.Repository,
) gin.HandlerFunc {
	type req struct {
		Messages map[string]string `json:"messages" binding:"required"`
		Status   string            `json:"status"`
	}
	return func(c *gin.Context) {
		p, err := resolveProject(c, projects)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		localeCode := c.Param("locale")
		var body req
		if err := c.ShouldBindJSON(&body); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		status := body.Status
		if status == "" {
			status = string(translation.StatusNeedsReview)
		}
		// Resolve locale.
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

		// Ensure every key exists. UpsertKeys is idempotent.
		inputs := make([]keyapp.UpsertInput, 0, len(body.Messages))
		for name := range body.Messages {
			inputs = append(inputs, keyapp.UpsertInput{Name: name})
		}
		if _, err := keysUC.Execute(contextOf(c), p.ID, inputs); err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}

		actor := uuid.Nil
		if v, ok := c.Get(ctxKeyUserID); ok {
			actor, _ = v.(uuid.UUID)
		}
		tenantID, _ := c.Get(ctxKeyTenantID)

		// Apply translations one by one. Per-row failures are
		// reported alongside successes; the request completes either
		// way so a single bad key doesn't strand the rest.
		results := make([]gin.H, 0, len(body.Messages))
		failed := 0
		for name, value := range body.Messages {
			keyID, err := keysFind.FindByName(contextOf(c), p.ID, name)
			if err != nil {
				failed++
				results = append(results, gin.H{"key": name, "error": err.Error()})
				continue
			}
			out, err := uc.Execute(contextOf(c), translationapp.UpdateInput{
				KeyID:     keyID,
				LocaleID:  l.ID,
				Value:     value,
				Status:    status,
				UpdatedBy: actor,
			})
			if err != nil {
				failed++
				results = append(results, gin.H{"key": name, "error": err.Error()})
				continue
			}
			_ = audits.Append(contextOf(c), audit.Entry{
				TenantID:      tenantID.(uuid.UUID),
				TranslationID: out.ID,
				AfterValue:    out.Value,
				ChangedBy:     actor,
			})
			pub.Publish(p.ID, tenantID.(uuid.UUID), translationapp.Event{
				Type:    "translation.updated",
				Project: p.Slug.String(),
				Locale:  localeCode,
				Key:     name,
				Value:   out.Value,
				Status:  string(out.Status),
			})
			results = append(results, gin.H{"key": name, "id": out.ID.String()})
		}
		code := http.StatusOK
		if failed > 0 && failed == len(body.Messages) {
			code = http.StatusUnprocessableEntity
		}
		c.JSON(code, gin.H{
			"applied":  len(body.Messages) - failed,
			"failed":   failed,
			"results":  results,
		})
	}
}

// handleBundleDiff — GET /api/v1/admin/projects/:slug/diff
//
// Returns per-locale counts of untranslated / needs-review /
// approved keys so the admin diff view can render a snapshot at a
// glance. No drill-down; the admin SPA hits the bundle endpoint
// per locale for the per-key list.
func handleBundleDiff(
	projects project.Repository,
	locales locale.Repository,
	listBundle *translationapp.ListBundle,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := resolveProject(c, projects)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		all, err := locales.ListForProject(contextOf(c), p.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out := make([]gin.H, 0, len(all))
		for _, l := range all {
			entries, err := listBundle.Execute(contextOf(c), p.ID, l.ID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			pending, needsReview, approved := 0, 0, 0
			for _, e := range entries {
				switch e.Status {
				case translation.StatusApproved:
					approved++
				case translation.StatusNeedsReview:
					needsReview++
				default:
					pending++
				}
			}
			out = append(out, gin.H{
				"locale":      l.Code.String(),
				"label":       l.Label.String(),
				"total":       len(entries),
				"pending":     pending,
				"needsReview": needsReview,
				"approved":    approved,
			})
		}
		c.JSON(http.StatusOK, gin.H{"project": p.Slug.String(), "locales": out})
	}
}
