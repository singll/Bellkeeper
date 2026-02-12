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

// AutheliaAuth middleware validates Authelia headers
func AutheliaAuth(mode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.GetHeader("X-Remote-User")

		// In debug mode, allow requests without auth
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
			Email:    c.GetHeader("X-Remote-Email"),
			Name:     c.GetHeader("X-Remote-Name"),
			Groups:   parseGroups(c.GetHeader("X-Remote-Groups")),
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
