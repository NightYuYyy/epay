// Package middleware provides HTTP middleware including JWT authentication
// for admin and user API endpoints.
package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"epay/internal/config"
	adminSvc "epay/internal/service/admin"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AdminAuth returns a gin middleware that validates JWT tokens issued by the
// admin service and injects admin claims into the request context.
func AdminAuth(cfg *config.Config) gin.HandlerFunc {
	svc := adminSvc.NewService(nil, cfg)
	return func(c *gin.Context) {
		token, ok := extractBearerToken(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "missing or invalid Authorization header"})
			return
		}
		claims, err := svc.VerifyToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid or expired token"})
			return
		}
		if claims.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "admin role required"})
			return
		}
		c.Set("admin_id", claims.AdminID)
		c.Set("admin_username", claims.Username)
		c.Set("admin_role", claims.Role)
		c.Next()
	}
}

// userClaims mirrors handler/user.Claims but lives here to avoid a circular
// import. Only the fields needed by the middleware are present.
type userClaims struct {
	jwt.RegisteredClaims
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
}

// UserAuth validates user JWT tokens and injects user_id / email / role into
// the request context. Tokens are HS256-signed with cfg.JWT.Secret.
func UserAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := extractBearerToken(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "missing or invalid Authorization header"})
			return
		}
		var claims userClaims
		parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(cfg.JWT.Secret), nil
		})
		if err != nil || !parsed.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid or expired token"})
			return
		}
		if claims.Role != "user" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "user role required"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)
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
