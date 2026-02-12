package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Mask secret values
	for i := range settings {
		if settings[i].IsSecret {
			settings[i].Value = settings[i].MaskedValue()
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": settings})
}

func (h *SettingHandler) Get(c *gin.Context) {
	key := c.Param("key")

	setting, err := h.svc.Get(key)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "setting not found"})
		return
	}

	// Mask secret values
	if setting.IsSecret {
		setting.Value = setting.MaskedValue()
	}

	c.JSON(http.StatusOK, gin.H{"data": setting})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ValueType == "" {
		req.ValueType = "string"
	}

	if err := h.svc.Set(key, req.Value, req.ValueType, req.Category, req.Description, req.IsSecret); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}
