package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// UserInfo contains authenticated user information from Authelia
type UserInfo struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Groups   []string `json:"groups"`
}

// AutheliaAuth middleware validates Authelia headers or API Key
func AutheliaAuth(mode string, apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. API Key authentication (for internal service calls like n8n)
		// Use constant-time comparison to prevent timing attacks
		if apiKey != "" && subtle.ConstantTimeCompare([]byte(c.GetHeader("X-API-Key")), []byte(apiKey)) == 1 {
			c.Set("user", UserInfo{
				Username: "api-service",
				Email:    "api@internal",
				Name:     "API Service",
				Groups:   []string{"admins"},
			})
			c.Next()
			return
		}

		// 2. No-auth mode: authentication is intentionally disabled (e.g. trusted
		// intranet / VPN-only deployment, after removing Authelia forward_auth).
		// Inject a default admin user so handlers relying on user context still work.
		if mode == "noauth" {
			c.Set("user", UserInfo{
				Username: "anonymous",
				Email:    "anonymous@localhost",
				Name:     "Anonymous",
				Groups:   []string{"admins"},
			})
			c.Next()
			return
		}

		user := c.GetHeader("Remote-User")

		// 3. In debug mode, allow requests without auth
		if mode == "debug" && user == "" {
			user = "dev-user"
			c.Set("user", UserInfo{
				Username: user,
				Email:    "dev@localhost",
				Name:     "Developer",
				Groups:   []string{"admins"},
			})
			c.Next()
			return
		}

		if user == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Please login via Authelia",
			})
			return
		}

		userInfo := UserInfo{
			Username: user,
			Email:    c.GetHeader("Remote-Email"),
			Name:     c.GetHeader("Remote-Name"),
			Groups:   parseGroups(c.GetHeader("Remote-Groups")),
		}

		c.Set("user", userInfo)
		c.Next()
	}
}

// GetUser retrieves user info from context
func GetUser(c *gin.Context) *UserInfo {
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(UserInfo); ok {
			return &u
		}
	}
	return nil
}

func parseGroups(header string) []string {
	if header == "" {
		return []string{}
	}
	groups := strings.Split(header, ",")
	for i, g := range groups {
		groups[i] = strings.TrimSpace(g)
	}
	return groups
}
