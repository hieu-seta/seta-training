package jwtauth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Gin context keys for authenticated uid + role.
const (
	CtxUID  = "uid"
	CtxRole = "role"
)

// Middleware validates Authorization: Bearer <token> + injects uid/role into ctx.
func Middleware(s *Signer) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		raw := strings.TrimPrefix(h, "Bearer ")
		claims, err := s.Parse(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(CtxUID, claims.UID)
		c.Set(CtxRole, claims.Role)
		c.Next()
	}
}

// RequireRole aborts 403 unless ctx role matches one of the given.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		r, _ := c.Get(CtxRole)
		s, _ := r.(string)
		if _, ok := allowed[s]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

// UID returns the authenticated user id from ctx (empty if absent).
func UID(c *gin.Context) string {
	v, _ := c.Get(CtxUID)
	s, _ := v.(string)
	return s
}

// Role returns the authenticated role from ctx (empty if absent).
func Role(c *gin.Context) string {
	v, _ := c.Get(CtxRole)
	s, _ := v.(string)
	return s
}
