package middleware

import (
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
		if apiKey != "" && c.GetHeader("X-API-Key") == apiKey {
			c.Set("user", UserInfo{
				Username: "api-service",
				Email:    "api@internal",
				Name:     "API Service",
				Groups:   []string{"admins"},
			})
			c.Next()
			return
		}

		user := c.GetHeader("Remote-User")

		// 2. In debug mode, allow requests without auth
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
