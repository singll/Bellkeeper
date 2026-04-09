package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/service"
)

// MatrixAdminHandler handles Matrix admin API requests
type MatrixAdminHandler struct {
	adminSvc *service.AdminService
}

// NewMatrixAdminHandler creates a new matrix admin handler
func NewMatrixAdminHandler(svc *service.AdminService) *MatrixAdminHandler {
	return &MatrixAdminHandler{adminSvc: svc}
}

// ListRooms handles GET /api/matrix/admin/rooms
func (h *MatrixAdminHandler) ListRooms(c *gin.Context) {
	rooms, err := h.adminSvc.ListRooms(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rooms": rooms})
}

// CreateRoom handles POST /api/matrix/admin/rooms
func (h *MatrixAdminHandler) CreateRoom(c *gin.Context) {
	var req struct {
		RoomID   string `json:"room_id" binding:"required"`
		Name     string `json:"name"`
		RoomType string `json:"room_type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.adminSvc.CreateRoom(c.Request.Context(), req.RoomID, req.Name, req.RoomType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "room created"})
}

// ListChannels handles GET /api/matrix/admin/channels
func (h *MatrixAdminHandler) ListChannels(c *gin.Context) {
	channels, err := h.adminSvc.ListChannels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"channels": channels})
}

// UpdateChannel handles PUT /api/matrix/admin/channels/:name
func (h *MatrixAdminHandler) UpdateChannel(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "channel name required"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.adminSvc.UpdateChannel(c.Request.Context(), name, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "channel updated"})
}

// ListCommands handles GET /api/matrix/admin/commands
func (h *MatrixAdminHandler) ListCommands(c *gin.Context) {
	commands, err := h.adminSvc.ListCommands(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"commands": commands})
}

// GetEventLogs handles GET /api/matrix/admin/events
func (h *MatrixAdminHandler) GetEventLogs(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	logs, err := h.adminSvc.GetEventLogs(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": logs})
}

// GetNotificationLogs handles GET /api/matrix/admin/notifications
func (h *MatrixAdminHandler) GetNotificationLogs(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	logs, err := h.adminSvc.GetNotificationLogs(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"notifications": logs})
}

// GetStats handles GET /api/matrix/admin/stats
func (h *MatrixAdminHandler) GetStats(c *gin.Context) {
	stats, err := h.adminSvc.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}
