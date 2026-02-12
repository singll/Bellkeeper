package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/service"
)

type WorkflowHandler struct {
	svc *service.WorkflowService
}

func NewWorkflowHandler(svc *service.WorkflowService) *WorkflowHandler {
	return &WorkflowHandler{svc: svc}
}

func (h *WorkflowHandler) Status(c *gin.Context) {
	status, err := h.svc.Status()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": status})
}

func (h *WorkflowHandler) Trigger(c *gin.Context) {
	name := c.Param("name")

	var payload map[string]interface{}
	c.ShouldBindJSON(&payload)

	result, err := h.svc.Trigger(name, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "data": result})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}
