package httpgin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authapp "github.com/felixgeelhaar/glossa/apps/api/internal/app/auth"
	"github.com/felixgeelhaar/glossa/apps/api/internal/domain/tenant"
)

type loginReq struct {
	TenantSlug string `json:"tenantSlug" binding:"required"`
	Email      string `json:"email" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

// handleLogin is the JWT issuance endpoint. POST /api/v1/auth/login.
// Translator clients hit it once per session; the admin SPA
// persists the returned token to localStorage.
//
// Tenant resolution: callers supply the tenant slug. We avoid
// host-based tenant inference for now because single-domain
// deploys (the default Glossa shape) need the explicit pick.
func handleLogin(uc *authapp.Login, tenants tenant.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		slug, err := tenant.NewSlug(req.TenantSlug)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		t, err := tenants.FindBySlug(c.Request.Context(), slug)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		out, err := uc.Execute(c.Request.Context(), authapp.LoginInput{
			TenantID: t.ID,
			Email:    req.Email,
			Password: req.Password,
		})
		if err != nil {
			if errors.Is(err, authapp.ErrInvalidCredentials) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"token":   out.Token,
			"expires": out.Expires,
			"user": gin.H{
				"id":      out.User.ID.String(),
				"email":   out.User.Email,
				"role":    string(out.User.Role),
				"locales": out.User.Locales,
			},
			"tenant": gin.H{
				"id":   t.ID.String(),
				"slug": t.Slug.String(),
				"name": t.Name.String(),
			},
		})
	}
}

// handleMe returns the JWT-resolved current user. The admin SPA
// calls this on boot to verify the stored token before showing
// the editor.
func handleMe() gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := authedUserID(c)
		role, _ := c.Get(ctxKeyUserRole)
		email, _ := c.Get(ctxKeyUserEmail)
		tenantID, _ := c.Get(ctxKeyTenantID)
		c.JSON(http.StatusOK, gin.H{
			"id":       uid.String(),
			"email":    email,
			"role":     role,
			"locales":  authedUserLocales(c),
			"tenantId": tenantID.(uuid.UUID).String(),
		})
	}
}
