package middleware

import (
	"net/http"
	"strings"

	systemauth "hephaestus-backend/pkg/auth"

	"github.com/gin-gonic/gin"
)

// MCPAuthMiddleware authenticates external MCP clients with Basic Auth.
// It also accepts the existing Bearer token so the authenticated admin console
// can keep using the built-in MCP debugger without storing the user's password.
func MCPAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))

		if strings.HasPrefix(authHeader, "Bearer ") {
			username, ok := authenticateBearerToken(authHeader)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
			c.Set("username", username)
			c.Next()
			return
		}

		username, password, ok := c.Request.BasicAuth()
		if !ok {
			writeBasicAuthRequired(c)
			return
		}

		user, valid, err := systemauth.VerifyCredentials(username, password)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "认证服务不可用"})
			return
		}
		if !valid {
			writeBasicAuthRequired(c)
			return
		}

		c.Set("username", user.Username)
		c.Set("role", user.Role)
		c.Next()
	}
}

func writeBasicAuthRequired(c *gin.Context) {
	c.Header("WWW-Authenticate", `Basic realm="Hephaestus MCP"`)
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid basic auth credentials"})
}
