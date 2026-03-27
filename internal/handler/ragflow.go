package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

type RagFlowHandler struct {
	svc *service.RagFlowService
}

func NewRagFlowHandler(svc *service.RagFlowService) *RagFlowHandler {
	return &RagFlowHandler{svc: svc}
}

func (h *RagFlowHandler) Upload(c *gin.Context) {
	var req service.UploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, err := h.svc.Upload(&req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, resp)
}

func (h *RagFlowHandler) UploadWithRouting(c *gin.Context) {
	var req service.UploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	resp, datasetID, err := h.svc.UploadWithRouting(&req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":       resp,
		"dataset_id": datasetID,
	})
}

func (h *RagFlowHandler) IngestObsidianNote(c *gin.Context) {
	var req service.ObsidianIngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Vault == "" || req.Path == "" || req.Content == "" {
		response.BadRequest(c, "vault, path and content are required")
		return
	}

	result, err := h.svc.IngestObsidianNote(&req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *RagFlowHandler) CheckURL(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		response.BadRequest(c, "url parameter required")
		return
	}

	normalize := c.DefaultQuery("normalize", "true") == "true"

	result, err := h.svc.CheckURLEnhanced(url, normalize)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *RagFlowHandler) ListDocuments(c *gin.Context) {
	datasetID := c.Query("dataset_id")
	if datasetID == "" {
		response.BadRequest(c, "dataset_id parameter required")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	// Proxy passthrough: forward RagFlow API response directly
	result, err := h.svc.ListDocuments(datasetID, page, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *RagFlowHandler) DeleteDocument(c *gin.Context) {
	documentID := c.Param("id")
	datasetID := c.Query("dataset_id")
	if datasetID == "" {
		response.BadRequest(c, "dataset_id parameter required")
		return
	}

	if err := h.svc.DeleteDocument(datasetID, documentID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Deleted(c)
}

// --- Batch B: RagFlow 高级操作 ---

// ListDatasets lists all RagFlow datasets (proxy passthrough)
func (h *RagFlowHandler) ListDatasets(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := h.svc.ListDatasets(page, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetDataset gets a single dataset (proxy passthrough)
func (h *RagFlowHandler) GetDataset(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	result, err := h.svc.GetDataset(datasetID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// CreateDataset creates a new RagFlow dataset (proxy passthrough)
func (h *RagFlowHandler) CreateDataset(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	name, ok := req["name"].(string)
	if !ok || name == "" {
		response.BadRequest(c, "name is required")
		return
	}
	delete(req, "name")

	result, err := h.svc.CreateDataset(name, req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, result)
}

// UpdateDataset updates a RagFlow dataset (proxy passthrough)
func (h *RagFlowHandler) UpdateDataset(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.UpdateDataset(datasetID, req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// DeleteDataset deletes a RagFlow dataset
func (h *RagFlowHandler) DeleteDataset(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	if err := h.svc.DeleteDataset(datasetID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Deleted(c)
}

// RunParsing triggers document parsing (proxy passthrough)
func (h *RagFlowHandler) RunParsing(c *gin.Context) {
	var req struct {
		DatasetID   string   `json:"dataset_id" binding:"required"`
		DocumentIDs []string `json:"document_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.RunParsing(req.DatasetID, req.DocumentIDs)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// RunParsingThrottled submits documents for parsing in rate-controlled batches.
func (h *RagFlowHandler) RunParsingThrottled(c *gin.Context) {
	var req struct {
		DatasetID       string   `json:"dataset_id" binding:"required"`
		DocumentIDs     []string `json:"document_ids" binding:"required"`
		BatchSize       int      `json:"batch_size"`
		IntervalSeconds int      `json:"interval_seconds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	h.svc.RunParsingThrottled(req.DatasetID, req.DocumentIDs, req.BatchSize, req.IntervalSeconds)

	c.JSON(http.StatusOK, gin.H{
		"message":          "throttled parsing started",
		"total_documents":  len(req.DocumentIDs),
		"batch_size":       req.BatchSize,
		"interval_seconds": req.IntervalSeconds,
	})
}

// RunParsingSmart starts a smart parsing queue with adaptive batching and retry.
func (h *RagFlowHandler) RunParsingSmart(c *gin.Context) {
	var req struct {
		Items  []service.ParseQueueItem  `json:"items" binding:"required"`
		Config service.ParseQueueConfig `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	total := 0
	for _, item := range req.Items {
		total += len(item.DocumentIDs)
	}

	taskID := h.svc.RunParsingQueue(req.Items, req.Config)

	c.JSON(http.StatusOK, gin.H{
		"task_id":         taskID,
		"message":         "smart parsing accepted and tracked until terminal result",
		"total_documents": total,
	})
}

// GetParseTaskStatus returns the status of a smart parsing task.
func (h *RagFlowHandler) GetParseTaskStatus(c *gin.Context) {
	taskID := c.Param("task_id")
	task := h.svc.GetParseTask(taskID)
	if task == nil {
		response.NotFound(c, "task not found")
		return
	}
	c.JSON(http.StatusOK, task)
}

// ListParseTasks returns all tracked parse tasks.
func (h *RagFlowHandler) ListParseTasks(c *gin.Context) {
	tasks := h.svc.ListParseTasks()
	response.Success(c, tasks)
}

// StopParsing stops document parsing (proxy passthrough)
func (h *RagFlowHandler) StopParsing(c *gin.Context) {
	var req struct {
		DatasetID   string   `json:"dataset_id" binding:"required"`
		DocumentIDs []string `json:"document_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.StopParsing(req.DatasetID, req.DocumentIDs)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetParsingStatus gets document parsing status (proxy passthrough)
func (h *RagFlowHandler) GetParsingStatus(c *gin.Context) {
	datasetID := c.Query("dataset_id")
	documentID := c.Query("document_id")
	if datasetID == "" || documentID == "" {
		response.BadRequest(c, "dataset_id and document_id parameters required")
		return
	}

	result, err := h.svc.GetParsingStatus(datasetID, documentID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// BatchUpload uploads multiple documents
func (h *RagFlowHandler) BatchUpload(c *gin.Context) {
	var req struct {
		DatasetID string                  `json:"dataset_id" binding:"required"`
		Documents []service.UploadRequest `json:"documents" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	results, errors := h.svc.BatchUpload(req.DatasetID, req.Documents)
	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"errors":  errors,
	})
}

// BatchDeleteDocuments deletes multiple documents
func (h *RagFlowHandler) BatchDeleteDocuments(c *gin.Context) {
	var req struct {
		DatasetID   string   `json:"dataset_id" binding:"required"`
		DocumentIDs []string `json:"document_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.svc.BatchDeleteDocuments(req.DatasetID, req.DocumentIDs); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("deleted %d documents", len(req.DocumentIDs)),
	})
}

// TransferDocument transfers a document between datasets
func (h *RagFlowHandler) TransferDocument(c *gin.Context) {
	var req struct {
		SourceDatasetID string `json:"source_dataset_id" binding:"required"`
		TargetDatasetID string `json:"target_dataset_id" binding:"required"`
		DocumentID      string `json:"document_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.TransferDocument(req.SourceDatasetID, req.TargetDatasetID, req.DocumentID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// UpdateDocumentMetadata updates document metadata (proxy passthrough)
func (h *RagFlowHandler) UpdateDocumentMetadata(c *gin.Context) {
	var req struct {
		DatasetID  string                 `json:"dataset_id" binding:"required"`
		DocumentID string                 `json:"document_id" binding:"required"`
		Metadata   map[string]interface{} `json:"metadata" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.UpdateDocumentMetadata(req.DatasetID, req.DocumentID, req.Metadata)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// UpdateDocumentParserConfig updates document parser method and parser_config.
func (h *RagFlowHandler) UpdateDocumentParserConfig(c *gin.Context) {
	var req struct {
		DatasetID    string                 `json:"dataset_id" binding:"required"`
		DocumentID   string                 `json:"document_id" binding:"required"`
		ParserID     string                 `json:"parser_id"`
		ParserConfig map[string]interface{} `json:"parser_config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.ParserID == "" && len(req.ParserConfig) == 0 {
		response.BadRequest(c, "parser_id or parser_config required")
		return
	}

	result, err := h.svc.UpdateDocumentParserConfig(req.DatasetID, req.DocumentID, req.ParserID, req.ParserConfig)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// ListChunks lists chunks for a document (proxy passthrough)
func (h *RagFlowHandler) ListChunks(c *gin.Context) {
	datasetID := c.Query("dataset_id")
	documentID := c.Query("document_id")
	if datasetID == "" || documentID == "" {
		response.BadRequest(c, "dataset_id and document_id parameters required")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := h.svc.ListChunks(datasetID, documentID, page, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// DeleteChunks deletes specific chunks (proxy passthrough)
func (h *RagFlowHandler) DeleteChunks(c *gin.Context) {
	var req struct {
		DatasetID  string   `json:"dataset_id" binding:"required"`
		DocumentID string   `json:"document_id" binding:"required"`
		ChunkIDs   []string `json:"chunk_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.DeleteChunks(req.DatasetID, req.DocumentID, req.ChunkIDs)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// BatchTransferDocuments transfers multiple documents between datasets
func (h *RagFlowHandler) BatchTransferDocuments(c *gin.Context) {
	var req struct {
		SourceDatasetID string   `json:"source_dataset_id" binding:"required"`
		TargetDatasetID string   `json:"target_dataset_id" binding:"required"`
		DocumentIDs     []string `json:"document_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.BatchTransferDocuments(req.SourceDatasetID, req.TargetDatasetID, req.DocumentIDs)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

// SyncDatasets synchronizes local dataset mappings with RAGFlow,
// auto-matching or auto-creating datasets to fill in missing UUIDs.
func (h *RagFlowHandler) SyncDatasets(c *gin.Context) {
	result, err := h.svc.SyncDatasetMappings()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "dataset sync completed",
		"result":  result,
	})
}

