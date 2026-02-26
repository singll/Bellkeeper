package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/service"
	"gorm.io/datatypes"
)

type WebhookHandler struct {
	svc *service.WebhookService
}

type WebhookRequest struct {
	Name           string                 `json:"name" binding:"required"`
	URL            string                 `json:"url" binding:"required"`
	Method         string                 `json:"method"`
	ContentType    string                 `json:"content_type"`
	Headers        map[string]interface{} `json:"headers"`
	BodyTemplate   string                 `json:"body_template"`
	TimeoutSeconds int                    `json:"timeout_seconds"`
	Description    string                 `json:"description"`
	IsActive       *bool                  `json:"is_active"`
}

func NewWebhookHandler(svc *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{svc: svc}
}

func (h *WebhookHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	webhooks, total, err := h.svc.List(page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     webhooks,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *WebhookHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	webhook, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": webhook})
}

func (h *WebhookHandler) Create(c *gin.Context) {
	var req WebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var headersJSON datatypes.JSON
	if req.Headers != nil {
		data, _ := json.Marshal(req.Headers)
		headersJSON = datatypes.JSON(data)
	}

	webhook := &model.WebhookConfig{
		Name:           req.Name,
		URL:            req.URL,
		Method:         req.Method,
		ContentType:    req.ContentType,
		Headers:        headersJSON,
		BodyTemplate:   req.BodyTemplate,
		TimeoutSeconds: req.TimeoutSeconds,
		Description:    req.Description,
		IsActive:       isActive,
	}

	if webhook.Method == "" {
		webhook.Method = "POST"
	}
	if webhook.ContentType == "" {
		webhook.ContentType = "application/json"
	}
	if webhook.TimeoutSeconds == 0 {
		webhook.TimeoutSeconds = 30
	}

	if err := h.svc.Create(webhook); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": webhook})
}

func (h *WebhookHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	webhook, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}

	var req WebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	webhook.Name = req.Name
	webhook.URL = req.URL
	if req.Method != "" {
		webhook.Method = req.Method
	}
	if req.ContentType != "" {
		webhook.ContentType = req.ContentType
	}
	if req.Headers != nil {
		data, _ := json.Marshal(req.Headers)
		webhook.Headers = datatypes.JSON(data)
	}
	webhook.BodyTemplate = req.BodyTemplate
	if req.TimeoutSeconds > 0 {
		webhook.TimeoutSeconds = req.TimeoutSeconds
	}
	webhook.Description = req.Description
	if req.IsActive != nil {
		webhook.IsActive = *req.IsActive
	}

	if err := h.svc.Update(webhook); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": webhook})
}

func (h *WebhookHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.svc.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *WebhookHandler) Trigger(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var payload map[string]interface{}
	c.ShouldBindJSON(&payload)

	history, err := h.svc.Trigger(uint(id), payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "history": history})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": history})
}

func (h *WebhookHandler) History(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	history, err := h.svc.GetHistory(uint(id), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": history})
}
