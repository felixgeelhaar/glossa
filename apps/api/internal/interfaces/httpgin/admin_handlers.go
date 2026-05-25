// Admin-only handlers — guarded by `jwtAuth` + `requireAdmin` at
// the route group. The translation-edit handler is NOT admin-only;
// translators can hit it with their JWT, but the audit-log /
// locale CRUD / user mgmt / project mgmt surfaces all need admin.

package httpgin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/auth"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/audit"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/locale"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/project"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/user"
)

// ─── Locales ─────────────────────────────────────────────────────

func handleSetLocaleEnabled(repo locale.Repository) gin.HandlerFunc {
	type req struct {
		Enabled bool `json:"enabled"`
	}
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("locale"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid locale id"})
			return
		}
		var body req
		if err := c.ShouldBindJSON(&body); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := repo.SetEnabled(c.Request.Context(), id, body.Enabled); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": id.String(), "enabled": body.Enabled})
	}
}

func handleDeleteLocale(repo locale.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("locale"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid locale id"})
			return
		}
		if err := repo.Delete(c.Request.Context(), id); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// ─── Users ──────────────────────────────────────────────────────

func handleListUsers(repo user.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, _ := c.Get(ctxKeyTenantID)
		users, err := repo.ListForTenant(c.Request.Context(), tenantID.(uuid.UUID))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out := make([]gin.H, 0, len(users))
		for _, u := range users {
			out = append(out, gin.H{
				"id":      u.ID.String(),
				"email":   u.Email,
				"role":    string(u.Role),
				"locales": u.Locales,
			})
		}
		c.JSON(http.StatusOK, out)
	}
}

func handleCreateUser(repo user.Repository) gin.HandlerFunc {
	type req struct {
		Email    string   `json:"email" binding:"required"`
		Password string   `json:"password" binding:"required"`
		Role     string   `json:"role" binding:"required"`
		Locales  []string `json:"locales"`
	}
	return func(c *gin.Context) {
		var body req
		if err := c.ShouldBindJSON(&body); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		email, err := user.NormalizeEmail(body.Email)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		role, err := user.ParseRole(body.Role)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		hash, err := authapp.HashPassword(body.Password)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		tenantID, _ := c.Get(ctxKeyTenantID)
		u, err := repo.Save(c.Request.Context(), user.User{
			TenantID:     tenantID.(uuid.UUID),
			Email:        email,
			PasswordHash: hash,
			Role:         role,
			Locales:      body.Locales,
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"id":      u.ID.String(),
			"email":   u.Email,
			"role":    string(u.Role),
			"locales": u.Locales,
		})
	}
}

func handleUpdateUserLocales(repo user.Repository) gin.HandlerFunc {
	type req struct {
		Locales []string `json:"locales"`
	}
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}
		var body req
		if err := c.ShouldBindJSON(&body); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := repo.UpdateLocales(c.Request.Context(), id, body.Locales); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": id.String(), "locales": body.Locales})
	}
}

func handleDeleteUser(repo user.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}
		// Guard against locking the tenant out by deleting the last
		// admin. Counted server-side under the same tx so the check
		// is race-safe.
		tenantID, _ := c.Get(ctxKeyTenantID)
		target, err := repo.FindByID(c.Request.Context(), id)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		if target.Role == user.RoleAdmin {
			count, err := repo.CountAdmins(c.Request.Context(), tenantID.(uuid.UUID))
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if count <= 1 {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "refuse to delete the last admin"})
				return
			}
		}
		if err := repo.Delete(c.Request.Context(), id); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// ─── Projects (admin) ───────────────────────────────────────────

func handleListProjects(repo project.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, _ := c.Get(ctxKeyTenantID)
		projects, err := repo.ListForTenant(c.Request.Context(), tenantID.(uuid.UUID))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out := make([]gin.H, 0, len(projects))
		for _, p := range projects {
			out = append(out, gin.H{
				"id":            p.ID.String(),
				"slug":          p.Slug.String(),
				"name":          p.Name.String(),
				"defaultLocale": p.DefaultLocale,
			})
		}
		c.JSON(http.StatusOK, out)
	}
}

// ─── Audit log ───────────────────────────────────────────────────

func handleListAudit(repo audit.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, _ := c.Get(ctxKeyTenantID)
		limit, offset := parseLimitOffset(c)
		entries, err := repo.ListForTenant(c.Request.Context(), tenantID.(uuid.UUID), limit, offset)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out := make([]gin.H, 0, len(entries))
		for _, e := range entries {
			out = append(out, gin.H{
				"id":            e.ID,
				"translationId": orNil(e.TranslationID),
				"beforeValue":   e.BeforeValue,
				"afterValue":    e.AfterValue,
				"changedBy":     orNil(e.ChangedBy),
				"actorKind":     e.ActorKind,
				"actorLabel":    e.ActorLabel,
				"changedAt":     e.ChangedAt,
			})
		}
		c.JSON(http.StatusOK, out)
	}
}

func parseLimitOffset(c *gin.Context) (int32, int32) {
	limit, offset := int32(100), int32(0)
	if v := c.Query("limit"); v != "" {
		if n, err := atoi32(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := atoi32(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func atoi32(s string) (int32, error) {
	var n int32
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int32(ch-'0')
	}
	return n, nil
}

func orNil(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id.String()
}
