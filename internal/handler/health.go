package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/service"
)

type HealthHandler struct {
	svc *service.HealthService
}

func NewHealthHandler(svc *service.HealthService) *HealthHandler {
	return &HealthHandler{svc: svc}
}

func (h *HealthHandler) Check(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Check())
}

func (h *HealthHandler) Detailed(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Detailed())
}
