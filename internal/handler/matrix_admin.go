package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/matrix/policy"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/service"
)

// MatrixAdminHandler handles Matrix admin API requests
type MatrixAdminHandler struct {
	adminSvc     *service.AdminService
	matrixDomain string // e.g., "matrix.singll.net"
}

// NewMatrixAdminHandler creates a new matrix admin handler
func NewMatrixAdminHandler(svc *service.AdminService, matrixDomain string) *MatrixAdminHandler {
	return &MatrixAdminHandler{
		adminSvc:     svc,
		matrixDomain: matrixDomain,
	}
}

// getCurrentUserID constructs the Matrix user ID from the authenticated user
func (h *MatrixAdminHandler) getCurrentUserID(c *gin.Context) string {
	user := middleware.GetUser(c)
	if user == nil {
		return ""
	}
	// Assume username is the local part of Matrix ID
	return fmt.Sprintf("@%s:%s", user.Username, h.matrixDomain)
}

// isAdminOrOwner checks if current user is admin (group) or owner in the target room
func (h *MatrixAdminHandler) isAdminOrOwner(c *gin.Context, roomID string) bool {
	user := middleware.GetUser(c)
	if user == nil {
		return false
	}

	// Check if user is in admins group
	for _, g := range user.Groups {
		if g == "admins" {
			return true
		}
	}

	// Check if user is owner in the room
	matrixID := h.getCurrentUserID(c)
	if matrixID == "" {
		return false
	}

	role, err := h.adminSvc.GetUserRolePolicy(c.Request.Context(), matrixID, roomID)
	if err != nil || role == "" {
		return false
	}
	return role == policy.RoleOwner
}

// ListRooms handles GET /api/matrix/admin/rooms
func (h *MatrixAdminHandler) ListRooms(c *gin.Context) {
	rooms, err := h.adminSvc.ListRooms(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Return { data: [...] } format to match frontend expectations
	c.JSON(http.StatusOK, gin.H{"data": rooms})
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

// DeleteRoom handles DELETE /api/matrix/admin/rooms/:id
func (h *MatrixAdminHandler) DeleteRoom(c *gin.Context) {
	roomID := c.Param("id")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room id required"})
		return
	}

	if err := h.adminSvc.DeleteRoom(c.Request.Context(), roomID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "room deleted"})
}

// ListChannels handles GET /api/matrix/admin/channels
func (h *MatrixAdminHandler) ListChannels(c *gin.Context) {
	channels, err := h.adminSvc.ListChannels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": channels})
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
	c.JSON(http.StatusOK, gin.H{"data": commands})
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
	c.JSON(http.StatusOK, gin.H{"data": logs})
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
	c.JSON(http.StatusOK, gin.H{"data": logs})
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

// ============ User Role Management ============

// ListUserRoles handles GET /api/matrix/admin/roles
// Query params: room_id (optional)
func (h *MatrixAdminHandler) ListUserRoles(c *gin.Context) {
	roomID := c.Query("room_id")
	if roomID != "" {
		roles, err := h.adminSvc.ListUserRoles(c.Request.Context(), roomID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": roles})
		return
	}

	// List all roles with pagination
	limit := 100
	offset := 0
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	roles, total, err := h.adminSvc.ListAllUserRoles(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": roles, "total": total})
}

// GetUserRole handles GET /api/matrix/admin/roles/:user_id
// Query params: room_id (required)
func (h *MatrixAdminHandler) GetUserRole(c *gin.Context) {
	userID := c.Param("user_id")
	roomID := c.Query("room_id")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room_id required"})
		return
	}

	role, err := h.adminSvc.GetUserRole(c.Request.Context(), userID, roomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if role == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"role": role})
}

// SetUserRole handles POST /api/matrix/admin/roles
// Requires admin group or room owner permission
func (h *MatrixAdminHandler) SetUserRole(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
		RoomID string `json:"room_id" binding:"required"`
		Role   string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Permission check: must be admin group or room owner
	if !h.isAdminOrOwner(c, req.RoomID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied: requires admin group or room owner"})
		return
	}

	// Validate role
	validRoles := map[string]bool{"owner": true, "admin": true, "member": true, "guest": true}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role, must be one of: owner, admin, member, guest"})
		return
	}

	if err := h.adminSvc.SetUserRole(c.Request.Context(), req.UserID, req.RoomID, req.Role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "role set"})
}

// RemoveUserRole handles DELETE /api/matrix/admin/roles/:user_id
// Query params: room_id (required)
// Requires admin group or room owner permission
func (h *MatrixAdminHandler) RemoveUserRole(c *gin.Context) {
	userID := c.Param("user_id")
	roomID := c.Query("room_id")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room_id required"})
		return
	}

	// Permission check: must be admin group or room owner
	if !h.isAdminOrOwner(c, roomID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied: requires admin group or room owner"})
		return
	}

	if err := h.adminSvc.RemoveUserRole(c.Request.Context(), userID, roomID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "role removed"})
}

// ListCommandLogs handles GET /api/matrix/admin/command-logs
func (h *MatrixAdminHandler) ListCommandLogs(c *gin.Context) {
	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	command := c.Query("command")
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	logs, total, err := h.adminSvc.ListCommandLogs(c.Request.Context(), page, perPage, command, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     logs,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}
