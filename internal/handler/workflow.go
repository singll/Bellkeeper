package handler

import (
	"net/http"
	"os"
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

func (h *WorkflowHandler) Definitions(c *gin.Context) {
	definitions, err := h.svc.ListWorkflowDefinitions()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, definitions)
}

func (h *WorkflowHandler) Definition(c *gin.Context) {
	key := c.Param("key")

	definition, err := h.svc.GetWorkflowDefinition(key)
	if err != nil {
		if os.IsNotExist(err) {
			response.NotFound(c, "workflow definition not found")
			return
		}
		response.BadRequest(c, err.Error())
		return
	}

	response.Success(c, definition)
}

func (h *WorkflowHandler) SaveDefinition(c *gin.Context) {
	key := c.Param("key")

	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, "invalid workflow definition JSON: "+err.Error())
		return
	}

	definition, err := h.svc.SaveWorkflowDefinition(key, payload)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Success(c, definition)
}

func (h *WorkflowHandler) DeleteDefinition(c *gin.Context) {
	key := c.Param("key")

	if err := h.svc.DeleteWorkflowDefinition(key); err != nil {
		if os.IsNotExist(err) {
			response.NotFound(c, "workflow definition not found")
			return
		}
		response.BadRequest(c, err.Error())
		return
	}

	response.Message(c, "workflow definition deleted")
}

func (h *WorkflowHandler) PushDefinition(c *gin.Context) {
	key := c.Param("key")

	result, err := h.svc.PushWorkflowDefinition(key)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *WorkflowHandler) PushAllDefinitions(c *gin.Context) {
	results, err := h.svc.PushAllWorkflowDefinitions()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, results)
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
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.Trigger(name, payload)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, result)
}
