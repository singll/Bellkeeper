package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

type SettingHandler struct {
	svc *service.SettingService
}

func NewSettingHandler(svc *service.SettingService) *SettingHandler {
	return &SettingHandler{svc: svc}
}

func (h *SettingHandler) List(c *gin.Context) {
	category := c.Query("category")

	settings, err := h.svc.List(category)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Mask secret values
	for i := range settings {
		if settings[i].IsSecret {
			settings[i].Value = settings[i].MaskedValue()
		}
	}

	response.Success(c, settings)
}

func (h *SettingHandler) Get(c *gin.Context) {
	key := c.Param("key")

	setting, err := h.svc.Get(key)
	if err != nil {
		response.NotFound(c, "setting not found")
		return
	}

	// Mask secret values
	if setting.IsSecret {
		setting.Value = setting.MaskedValue()
	}

	response.Success(c, setting)
}

type UpdateSettingRequest struct {
	Value       string `json:"value" binding:"required"`
	ValueType   string `json:"value_type"`
	Category    string `json:"category"`
	Description string `json:"description"`
	IsSecret    bool   `json:"is_secret"`
}

func (h *SettingHandler) Update(c *gin.Context) {
	key := c.Param("key")

	var req UpdateSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.ValueType == "" {
		req.ValueType = "string"
	}

	if err := h.svc.Set(key, req.Value, req.ValueType, req.Category, req.Description, req.IsSecret); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Message(c, "updated")
}
