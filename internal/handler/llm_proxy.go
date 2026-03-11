package handler

import (
	"io"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

type LLMProxyHandler struct {
	svc *service.LLMProxyService
}

func NewLLMProxyHandler(svc *service.LLMProxyService) *LLMProxyHandler {
	return &LLMProxyHandler{svc: svc}
}

// Proxy handles OpenAI-compatible proxy requests.
// Route: Any /api/llm/v1/*path
func (h *LLMProxyHandler) Proxy(c *gin.Context) {
	path := "/v1" + c.Param("path")
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "failed to read request body")
		return
	}

	callerID := c.GetHeader("X-Caller-ID")
	if callerID == "" {
		callerID = "unknown"
	}

	statusCode, respBody, respHeaders, err := h.svc.ProxyRequest(
		c.Request.Method, path, c.Request.Header, body, callerID,
	)

	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Forward upstream content-type
	if ct := respHeaders.Get("Content-Type"); ct != "" {
		c.Header("Content-Type", ct)
	}

	c.Data(statusCode, "application/json", respBody)
}

// ChannelsStatus returns current status of all channels.
func (h *LLMProxyHandler) ChannelsStatus(c *gin.Context) {
	response.Success(c, h.svc.GetChannelsStatus())
}

// Stats returns aggregated proxy statistics.
func (h *LLMProxyHandler) Stats(c *gin.Context) {
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 {
		hours = 24
	}
	stats, err := h.svc.GetStats(hours)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, stats)
}

// Logs returns recent proxy request logs.
func (h *LLMProxyHandler) Logs(c *gin.Context) {
	channel := c.Query("channel")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}
	logs, err := h.svc.GetRecentLogs(channel, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, logs)
}

// RateLimitEvents returns recent rate-limit events.
func (h *LLMProxyHandler) RateLimitEvents(c *gin.Context) {
	channel := c.Query("channel")
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 {
		hours = 24
	}
	events, err := h.svc.GetRateLimitEvents(hours, channel)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, events)
}

// HealthStatus returns health status of all channels including circuit breaker state.
func (h *LLMProxyHandler) HealthStatus(c *gin.Context) {
	response.Success(c, h.svc.GetHealthStatus())
}

// GroupsStatus returns status of all virtual model groups.
func (h *LLMProxyHandler) GroupsStatus(c *gin.Context) {
	response.Success(c, h.svc.GetGroupsStatus())
}

// ClearGroupSticky clears all sticky bindings for the named model group.
func (h *LLMProxyHandler) ClearGroupSticky(c *gin.Context) {
	name := c.Param("name")
	cleared := h.svc.ClearGroupSticky(name)
	if cleared < 0 {
		response.NotFound(c, "model group not found: "+name)
		return
	}
	response.Success(c, gin.H{"cleared": cleared})
}

// ResetChannelCircuit resets the circuit breaker for the named channel.
func (h *LLMProxyHandler) ResetChannelCircuit(c *gin.Context) {
	name := c.Param("name")
	if ok := h.svc.ResetChannelCircuit(name); !ok {
		response.NotFound(c, "channel not found: "+name)
		return
	}
	response.Message(c, "circuit breaker reset for channel: "+name)
}

// --- Channel & Group Config CRUD ---

func (h *LLMProxyHandler) ListChannels(c *gin.Context) {
	channels, err := h.svc.ListChannelConfigs()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, channels)
}

func (h *LLMProxyHandler) CreateChannel(c *gin.Context) {
	var ch model.LLMChannel
	if err := c.ShouldBindJSON(&ch); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.CreateChannel(&ch); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, ch)
}

func (h *LLMProxyHandler) UpdateChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var ch model.LLMChannel
	if err := c.ShouldBindJSON(&ch); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	ch.ID = uint(id)
	if err := h.svc.UpdateChannel(&ch); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, ch)
}

func (h *LLMProxyHandler) DeleteChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.svc.DeleteChannel(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "channel deleted")
}

func (h *LLMProxyHandler) ListGroups(c *gin.Context) {
	groups, err := h.svc.ListGroupConfigs()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, groups)
}

func (h *LLMProxyHandler) CreateGroup(c *gin.Context) {
	var g model.LLMModelGroup
	if err := c.ShouldBindJSON(&g); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.CreateGroup(&g); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, g)
}

func (h *LLMProxyHandler) UpdateGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var g model.LLMModelGroup
	if err := c.ShouldBindJSON(&g); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	g.ID = uint(id)
	if err := h.svc.UpdateGroup(&g); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, g)
}

func (h *LLMProxyHandler) DeleteGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.svc.DeleteGroup(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "group deleted")
}

func (h *LLMProxyHandler) ReloadConfig(c *gin.Context) {
	if err := h.svc.Reload(); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Message(c, "configuration reloaded")
}
