package handler

import (
	"crypto/subtle"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

type LogCenterHandler struct {
	svc    *service.LogCenterService
	apiKey string // server api_key for admin operations
}

func NewLogCenterHandler(svc *service.LogCenterService, apiKey string) *LogCenterHandler {
	return &LogCenterHandler{svc: svc, apiKey: apiKey}
}

func (h *LogCenterHandler) CreateEntry(c *gin.Context) {
	var req struct {
		Module     string `json:"module" binding:"required"`
		Action     string `json:"action" binding:"required"`
		Level      string `json:"level"`
		Status     string `json:"status"`
		Summary    string `json:"summary"`
		Detail     any    `json:"detail"`
		RefID      string `json:"ref_id"`
		DurationMs int    `json:"duration_ms"`
		TraceID    string `json:"trace_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// Default values
	level := req.Level
	if level == "" {
		level = "info"
	}
	status := req.Status
	if status == "" {
		status = "success"
	}

	// Resolve source: X-API-Key header for external sources, default to bellkeeper-core
	sourceID := uint(1) // default: bellkeeper-core
	if apiKeyHeader := c.GetHeader("X-API-Key"); apiKeyHeader != "" {
		source, err := h.svc.FindSourceByAPIKey(apiKeyHeader)
		if err == nil && source != nil {
			sourceID = source.ID
		}
	}

	h.svc.LogActivity(service.LogEntryParams{
		SourceID:   sourceID,
		Module:     req.Module,
		Action:     req.Action,
		Level:      level,
		Status:     status,
		Summary:    req.Summary,
		Detail:     req.Detail,
		RefID:      req.RefID,
		DurationMs: req.DurationMs,
		TraceID:    req.TraceID,
	})

	response.Success(c, gin.H{"message": "entry logged"})
}

func (h *LogCenterHandler) ListEntries(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	var sourceID uint
	if sid := c.Query("source_id"); sid != "" {
		if v, err := strconv.ParseUint(sid, 10, 32); err == nil {
			sourceID = uint(v)
		}
	}

	var since, until time.Time
	if s := c.Query("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}
	if s := c.Query("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			until = t
		}
	}

	result, err := h.svc.ListEntries(service.ListEntriesQuery{
		SourceID: sourceID,
		Module:   c.Query("module"),
		Level:    c.Query("level"),
		Status:   c.Query("status"),
		TraceID:  c.Query("trace_id"),
		Keyword:  c.Query("keyword"),
		Since:    since,
		Until:    until,
		Page:     page,
		Limit:    limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *LogCenterHandler) GetEntry(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}
	entry, err := h.svc.GetEntry(id)
	if err != nil {
		response.NotFound(c, "entry not found")
		return
	}
	response.Success(c, entry)
}

func (h *LogCenterHandler) ListSources(c *gin.Context) {
	sources, err := h.svc.ListSources()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, sources)
}

func (h *LogCenterHandler) RegisterSource(c *gin.Context) {
	// Admin only: requires server api_key
	apiKey := c.GetHeader("X-API-Key")
	if subtle.ConstantTimeCompare([]byte(apiKey), []byte(h.apiKey)) != 1 {
		response.Error(c, 401, "unauthorized: admin only")
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		SourceType  string `json:"source_type" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	source, err := h.svc.RegisterSource(req.Name, req.SourceType, req.Description)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	// Return API key only on initial registration (one-time display)
	response.Created(c, gin.H{
		"id":          source.ID,
		"name":        source.Name,
		"source_type": source.SourceType,
		"description": source.Description,
		"is_active":   source.IsActive,
		"created_at":  source.CreatedAt,
		"api_key":     source.APIKey, // shown only once
	})
}

func (h *LogCenterHandler) UpdateSource(c *gin.Context) {
	// Admin only: requires server api_key
	apiKey := c.GetHeader("X-API-Key")
	if subtle.ConstantTimeCompare([]byte(apiKey), []byte(h.apiKey)) != 1 {
		response.Error(c, 401, "unauthorized: admin only")
		return
	}

	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	var req struct {
		Name        *string `json:"name"`
		SourceType  *string `json:"source_type"`
		Description *string `json:"description"`
		IsActive    *bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	source, err := h.svc.UpdateSource(id, req.Name, req.SourceType, req.Description, req.IsActive)
	if err != nil {
		response.NotFound(c, "source not found")
		return
	}
	response.Success(c, source)
}

func (h *LogCenterHandler) DeleteSource(c *gin.Context) {
	// Admin only: requires server api_key
	apiKey := c.GetHeader("X-API-Key")
	if subtle.ConstantTimeCompare([]byte(apiKey), []byte(h.apiKey)) != 1 {
		response.Error(c, 401, "unauthorized: admin only")
		return
	}

	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	if err := h.svc.DeleteSource(id); err != nil {
		response.NotFound(c, "source not found")
		return
	}
	response.Deleted(c)
}

func (h *LogCenterHandler) GetDashboard(c *gin.Context) {
	var since time.Time
	if s := c.Query("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}
	dashboard, err := h.svc.GetDashboard(since)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, dashboard)
}

func (h *LogCenterHandler) GetDashboardByPeriod(c *gin.Context) {
	period := c.Param("period")
	dashboard, err := h.svc.GetDashboardByPeriod(period)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, dashboard)
}

func (h *LogCenterHandler) ListAlertRules(c *gin.Context) {
	rules, err := h.svc.ListAlertRules()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, rules)
}

func (h *LogCenterHandler) CreateAlertRule(c *gin.Context) {
	// Admin only: requires server api_key
	apiKey := c.GetHeader("X-API-Key")
	if subtle.ConstantTimeCompare([]byte(apiKey), []byte(h.apiKey)) != 1 {
		response.Error(c, 401, "unauthorized: admin only")
		return
	}

	var req struct {
		Name          string          `json:"name" binding:"required"`
		Condition     json.RawMessage `json:"condition" binding:"required"`
		NotifyChannel string          `json:"notify_channel"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	rule, err := h.svc.CreateAlertRule(req.Name, req.Condition, req.NotifyChannel)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, rule)
}

func (h *LogCenterHandler) UpdateAlertRule(c *gin.Context) {
	// Admin only: requires server api_key
	apiKey := c.GetHeader("X-API-Key")
	if subtle.ConstantTimeCompare([]byte(apiKey), []byte(h.apiKey)) != 1 {
		response.Error(c, 401, "unauthorized: admin only")
		return
	}

	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	var req struct {
		Name          *string         `json:"name"`
		Condition     json.RawMessage `json:"condition"`
		NotifyChannel *string         `json:"notify_channel"`
		IsActive      *bool           `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	rule, err := h.svc.UpdateAlertRule(id, req.Name, req.Condition, req.NotifyChannel, req.IsActive)
	if err != nil {
		response.NotFound(c, "alert rule not found")
		return
	}
	response.Success(c, rule)
}

func (h *LogCenterHandler) DeleteAlertRule(c *gin.Context) {
	// Admin only: requires server api_key
	apiKey := c.GetHeader("X-API-Key")
	if subtle.ConstantTimeCompare([]byte(apiKey), []byte(h.apiKey)) != 1 {
		response.Error(c, 401, "unauthorized: admin only")
		return
	}

	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	if err := h.svc.DeleteAlertRule(id); err != nil {
		response.NotFound(c, "alert rule not found")
		return
	}
	response.Deleted(c)
}