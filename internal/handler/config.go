package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/llmgateway"
	"github.com/singll/bellkeeper/internal/pkg/response"
)

// ConfigHandler handles configuration hot-reload requests
type ConfigHandler struct {
	llmProxy *llmgateway.LLMProxyService
}

// NewConfigHandler creates a new ConfigHandler
func NewConfigHandler(llmProxy *llmgateway.LLMProxyService) *ConfigHandler {
	return &ConfigHandler{llmProxy: llmProxy}
}

// ReloadLLMProxy reloads the LLM proxy configuration
func (h *ConfigHandler) ReloadLLMProxy(c *gin.Context) {
	if h.llmProxy == nil {
		response.NotFound(c, "LLM proxy not configured")
		return
	}

	if err := h.llmProxy.Reload(); err != nil {
		response.InternalError(c, "failed to reload LLM proxy: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "LLM proxy configuration reloaded",
	})
}

// ReloadAll triggers a full configuration reload
func (h *ConfigHandler) ReloadAll(c *gin.Context) {
	var results = make(map[string]string)

	// Reload LLM Proxy
	if h.llmProxy != nil {
		if err := h.llmProxy.Reload(); err != nil {
			results["llm_proxy"] = "error: " + err.Error()
		} else {
			results["llm_proxy"] = "ok"
		}
	}

	response.Success(c, gin.H{
		"message": "configuration reload complete",
		"results": results,
	})
}
