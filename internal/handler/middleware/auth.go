// Package middleware provides HTTP middleware including JWT authentication
// for admin and merchant API endpoints.
package middleware

import (
	"net/http"
	"strings"

	"epay/internal/config"
	adminSvc "epay/internal/service/admin"

	"github.com/gin-gonic/gin"
)

// AdminAuth returns a gin middleware that validates JWT tokens and injects
// admin claims into the request context. Returns 401 on invalid/expired tokens.
func AdminAuth(cfg *config.Config) gin.HandlerFunc {
	svc := adminSvc.NewService(nil, cfg)

	return func(c *gin.Context) {
		token, ok := extractBearerToken(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 401,
				"msg":  "missing or invalid Authorization header",
			})
			return
		}

		claims, err := svc.VerifyToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 401,
				"msg":  "invalid or expired token",
			})
			return
		}

		if claims.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": 403,
				"msg":  "admin role required",
			})
			return
		}

		// Inject admin info into context
		c.Set("admin_id", claims.AdminID)
		c.Set("admin_username", claims.Username)
		c.Set("admin_role", claims.Role)

		c.Next()
	}
}

// MerchantAuth returns a gin middleware that validates JWT tokens and injects
// merchant claims into the request context. Returns 401 on invalid/expired tokens.
func MerchantAuth(cfg *config.Config) gin.HandlerFunc {
	svc := adminSvc.NewService(nil, cfg)

	return func(c *gin.Context) {
		token, ok := extractBearerToken(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 401,
				"msg":  "missing or invalid Authorization header",
			})
			return
		}

		claims, err := svc.VerifyToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 401,
				"msg":  "invalid or expired token",
			})
			return
		}

		if claims.Role != "merchant" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": 403,
				"msg":  "merchant role required",
			})
			return
		}

		// Inject merchant info into context
		c.Set("merchant_id", claims.AdminID) // reusing AdminID for pid; merchant roles use Subject field
		c.Set("merchant_role", claims.Role)

		c.Next()
	}
}

// extractBearerToken extracts the Bearer token from the Authorization header.
func extractBearerToken(c *gin.Context) (string, bool) {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		return "", false
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}

	return token, true
}
