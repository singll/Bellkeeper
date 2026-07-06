package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/handler"
)

// TestLLMRouteRegistration verifies the LLM proxy route tree registers without a
// gin radix-tree conflict (sibling wildcards must share a param name) and that the
// Tier 6 credential + balance-history routes are present.
func TestLLMRouteRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	h := handler.NewLLMProxyHandler(nil, nil)

	// Panics here on a route conflict (caught a real :id/:channel_id clash once).
	registerLLMProxyRoutes(r, api, h, nil, "")

	got := make(map[string]bool)
	for _, ri := range r.Routes() {
		got[ri.Method+" "+ri.Path] = true
	}
	want := []string{
		"GET /api/llm/config/channels/:id/credentials",
		"POST /api/llm/config/channels/:id/credentials",
		"PUT /api/llm/config/credentials/:id",
		"DELETE /api/llm/config/credentials/:id",
		"GET /api/llm/channels/:name/balance/history",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("route not registered: %s", w)
		}
	}
}
