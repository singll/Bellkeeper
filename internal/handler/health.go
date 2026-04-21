package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

type HealthHandler struct {
	svc *service.HealthService
}

func NewHealthHandler(svc *service.HealthService) *HealthHandler {
	return &HealthHandler{svc: svc}
}

// Check is a basic liveness check
func (h *HealthHandler) Check(c *gin.Context) {
	response.Success(c, h.svc.Check())
}

// Detailed provides comprehensive health status
func (h *HealthHandler) Detailed(c *gin.Context) {
	response.Success(c, h.svc.Detailed())
}

// Liveness is a simple liveness probe that returns 200 if the service is running
func (h *HealthHandler) Liveness(c *gin.Context) {
	response.Success(c, gin.H{"status": "alive"})
}

// Readiness checks if the service is ready to accept traffic
func (h *HealthHandler) Readiness(c *gin.Context) {
	health := h.svc.Detailed()
	if health.Status == "healthy" {
		response.Success(c, health)
	} else {
		response.Error(c, http.StatusServiceUnavailable, "service not ready")
	}
}
