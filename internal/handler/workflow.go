package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
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
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, status)
}

func (h *WorkflowHandler) Get(c *gin.Context) {
	id := c.Param("id")

	workflow, err := h.svc.GetWorkflow(id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, workflow)
}

func (h *WorkflowHandler) Activate(c *gin.Context) {
	id := c.Param("id")

	if err := h.svc.ActivateWorkflow(id); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Message(c, "workflow activated")
}

func (h *WorkflowHandler) Deactivate(c *gin.Context) {
	id := c.Param("id")

	if err := h.svc.DeactivateWorkflow(id); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Message(c, "workflow deactivated")
}

func (h *WorkflowHandler) Executions(c *gin.Context) {
	workflowID := c.Query("workflow_id")
	limitStr := c.DefaultQuery("limit", "20")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 20
	}

	executions, err := h.svc.GetExecutions(workflowID, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, executions)
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

	response.Success(c, result)
}
