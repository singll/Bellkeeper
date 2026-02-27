package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SystemHandler handles system-level operations like restart
type SystemHandler struct {
	shutdownChan chan struct{}
}

// NewSystemHandler creates a new SystemHandler
func NewSystemHandler(shutdownChan chan struct{}) *SystemHandler {
	return &SystemHandler{shutdownChan: shutdownChan}
}

// Restart triggers a graceful server restart.
// Returns 202 Accepted immediately, then signals main to shut down.
// Docker's restart policy will bring the container back up.
func (h *SystemHandler) Restart(c *gin.Context) {
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Server is restarting...",
	})

	// Delay briefly to ensure the HTTP response is flushed to the client
	go func() {
		time.Sleep(500 * time.Millisecond)
		select {
		case h.shutdownChan <- struct{}{}:
		default:
			// Already shutting down
		}
	}()
}
