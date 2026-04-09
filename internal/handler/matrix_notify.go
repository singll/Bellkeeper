package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/service"
)

// MatrixNotifyHandler handles notification API requests
type MatrixNotifyHandler struct {
	svc *service.NotificationService
}

// NewMatrixNotifyHandler creates a new matrix notify handler
func NewMatrixNotifyHandler(svc *service.NotificationService) *MatrixNotifyHandler {
	return &MatrixNotifyHandler{svc: svc}
}

// Send handles POST /api/matrix/notify
func (h *MatrixNotifyHandler) Send(c *gin.Context) {
	var req service.NotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message":  "invalid request: " + err.Error(),
		})
		return
	}

	resp, err := h.svc.Send(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to queue notification: " + err.Error(),
		})
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, resp)
		return
	}

	c.JSON(http.StatusAccepted, resp)
}

// GetStatus handles GET /api/matrix/notify/:id
func (h *MatrixNotifyHandler) GetStatus(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message":  "notification id is required",
		})
		return
	}

	notification, err := h.svc.GetStatus(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message":  "notification not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"notification":   notification,
	})
}

// ListChannels handles GET /api/matrix/notify/channels
func (h *MatrixNotifyHandler) ListChannels(c *gin.Context) {
	channels := h.svc.GetChannels(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"channels": channels,
	})
}

// ReloadChannels handles POST /api/matrix/notify/channels/reload
func (h *MatrixNotifyHandler) ReloadChannels(c *gin.Context) {
	h.svc.ReloadChannels(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "channels reloaded",
	})
}
