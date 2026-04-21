package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
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
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	resp, err := h.svc.Send(c.Request.Context(), &req)
	if err != nil {
		response.InternalError(c, "failed to queue notification: "+err.Error())
		return
	}

	if !resp.Success {
		response.Error(c, http.StatusBadRequest, resp.Message)
		return
	}

	response.Raw(c, http.StatusAccepted, resp)
}

// GetStatus handles GET /api/matrix/notify/:id
func (h *MatrixNotifyHandler) GetStatus(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		response.BadRequest(c, "notification id is required")
		return
	}

	notification, err := h.svc.GetStatus(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "notification not found")
		return
	}

	response.Success(c, gin.H{"notification": notification})
}

// ListChannels handles GET /api/matrix/notify/channels
func (h *MatrixNotifyHandler) ListChannels(c *gin.Context) {
	channels := h.svc.GetChannels(c.Request.Context())
	response.Success(c, gin.H{"channels": channels})
}

// ReloadChannels handles POST /api/matrix/notify/channels/reload
func (h *MatrixNotifyHandler) ReloadChannels(c *gin.Context) {
	h.svc.ReloadChannels(c.Request.Context())
	response.Message(c, "channels reloaded")
}
