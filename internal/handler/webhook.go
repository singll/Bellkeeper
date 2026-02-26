package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/pkg/response"
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
	page, perPage := response.ParsePagination(c)

	webhooks, total, err := h.svc.List(page, perPage)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Page(c, webhooks, total, page, perPage)
}

func (h *WebhookHandler) Get(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	webhook, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "webhook not found")
		return
	}

	response.Success(c, webhook)
}

func (h *WebhookHandler) Create(c *gin.Context) {
	var req WebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var headersJSON datatypes.JSON
	if req.Headers != nil {
		data, err := json.Marshal(req.Headers)
		if err != nil {
			response.BadRequest(c, "invalid headers format")
			return
		}
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
		webhook.Method = defaults.DefaultWebhookMethod
	}
	if webhook.ContentType == "" {
		webhook.ContentType = defaults.DefaultWebhookContentType
	}
	if webhook.TimeoutSeconds == 0 {
		webhook.TimeoutSeconds = defaults.DefaultWebhookTimeout
	}

	if err := h.svc.Create(webhook); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, webhook)
}

func (h *WebhookHandler) Update(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	webhook, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "webhook not found")
		return
	}

	var req WebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
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
		data, err := json.Marshal(req.Headers)
		if err != nil {
			response.BadRequest(c, "invalid headers format")
			return
		}
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
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, webhook)
}

func (h *WebhookHandler) Delete(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	if err := h.svc.Delete(id); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Deleted(c)
}

func (h *WebhookHandler) Trigger(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	var req struct {
		Payload   map[string]interface{} `json:"payload"`
		Variables map[string]string      `json:"variables"`
	}
	c.ShouldBindJSON(&req)

	history, err := h.svc.TriggerWithVariables(id, req.Payload, req.Variables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "history": history})
		return
	}

	response.Success(c, history)
}

func (h *WebhookHandler) History(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")

	var history []model.WebhookHistory
	var err error
	if status != "" {
		history, err = h.svc.GetHistoryByStatus(id, status, limit)
	} else {
		history, err = h.svc.GetHistory(id, limit)
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, history)
}
