package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/pkg/response"
)

// LogLevelHandler handles log level management
type LogLevelHandler struct{}

// NewLogLevelHandler creates a new LogLevelHandler
func NewLogLevelHandler() *LogLevelHandler {
	return &LogLevelHandler{}
}

// GetLevel returns the current log level
func (h *LogLevelHandler) GetLevel(c *gin.Context) {
	level := middleware.GetLogLevel()
	response.Success(c, gin.H{
		"level": level,
	})
}

// SetLevel changes the log level dynamically
func (h *LogLevelHandler) SetLevel(c *gin.Context) {
	var req struct {
		Level string `json:"level" binding:"required,oneof=debug info warn error"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid level: must be one of debug, info, warn, error")
		return
	}

	if err := middleware.SetLogLevel(req.Level); err != nil {
		response.InternalError(c, "failed to set log level: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"level": req.Level,
		"message": "log level updated",
	})
}
