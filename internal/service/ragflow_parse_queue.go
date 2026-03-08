package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// --- Smart Parsing Queue ---

var parseTasks sync.Map // taskID -> *ParseTask

// ParseQueueItem represents a dataset and its documents to parse.
type ParseQueueItem struct {
	DatasetID   string   `json:"dataset_id"`
	DocumentIDs []string `json:"document_ids"`
}

// ParseQueueConfig controls parsing queue behavior.
type ParseQueueConfig struct {
	BatchSize     int    `json:"batch_size"`      // initial batch size (default 3)
	MaxRetries    int    `json:"max_retries"`     // per-doc max retries (default 3)
	InitialDelay  int    `json:"initial_delay"`   // seconds between batches (default 15)
	PollInterval  int    `json:"poll_interval"`   // seconds between status polls (default 10)
	PollTimeout   int    `json:"poll_timeout"`    // max seconds to wait for a batch (default 300)
	NotifyWebhook string `json:"notify_webhook"`  // B01 webhook URL for completion notification
	NotifyRoom    string `json:"notify_room"`     // Matrix room for notification
}

// ParseTask tracks the progress of a smart parsing queue.
type ParseTask struct {
	mu          sync.Mutex  `json:"-"`
	ID          string      `json:"id"`
	Status      string      `json:"status"` // running / completed
	Total       int         `json:"total"`
	Completed   int         `json:"completed"`
	Failed      int         `json:"failed"`
	Pending     int         `json:"pending"`
	BatchSize   int         `json:"batch_size"`
	FailedDocs  []FailedDoc `json:"failed_docs,omitempty"`
	StartedAt   time.Time   `json:"started_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	Log         []string    `json:"log,omitempty"`
}

// FailedDoc records a document that failed to parse.
type FailedDoc struct {
	DatasetID  string `json:"dataset_id"`
	DocumentID string `json:"document_id"`
	Error      string `json:"error"`
	Retries    int    `json:"retries"`
}

func (t *ParseTask) addLog(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	t.Log = append(t.Log, entry)
	log.Printf("info: [parse-queue %s] %s", t.ID, msg)
}

func (t *ParseTask) snapshot() ParseTask {
	t.mu.Lock()
	defer t.mu.Unlock()
	failedDocs := make([]FailedDoc, len(t.FailedDocs))
	copy(failedDocs, t.FailedDocs)
	logs := make([]string, len(t.Log))
	copy(logs, t.Log)
	return ParseTask{
		ID:          t.ID,
		Status:      t.Status,
		Total:       t.Total,
		Completed:   t.Completed,
		Failed:      t.Failed,
		Pending:     t.Pending,
		BatchSize:   t.BatchSize,
		FailedDocs:  failedDocs,
		StartedAt:   t.StartedAt,
		CompletedAt: t.CompletedAt,
		Log:         logs,
	}
}

// RunParsingQueue starts a smart parsing queue in the background.
// Returns the task ID for progress tracking.
func (s *RagFlowService) RunParsingQueue(items []ParseQueueItem, cfg ParseQueueConfig) string {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 3
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 15
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10
	}
	if cfg.PollTimeout <= 0 {
		cfg.PollTimeout = 300
	}

	totalDocs := 0
	for _, item := range items {
		totalDocs += len(item.DocumentIDs)
	}

	taskID := fmt.Sprintf("parse-%d", time.Now().UnixMilli())
	task := &ParseTask{
		ID:        taskID,
		Status:    "running",
		Total:     totalDocs,
		Pending:   totalDocs,
		BatchSize: cfg.BatchSize,
		StartedAt: time.Now(),
	}
	parseTasks.Store(taskID, task)

	go s.executeParsingQueue(task, items, cfg)
	return taskID
}

// GetParseTask returns a snapshot of the parse task, or nil if not found.
func (s *RagFlowService) GetParseTask(taskID string) *ParseTask {
	v, ok := parseTasks.Load(taskID)
	if !ok {
		return nil
	}
	task := v.(*ParseTask)
	snap := task.snapshot()
	return &snap
}

func (s *RagFlowService) executeParsingQueue(task *ParseTask, items []ParseQueueItem, cfg ParseQueueConfig) {
	defer func() {
		now := time.Now()
		task.mu.Lock()
		task.Status = "completed"
		task.CompletedAt = &now
		task.mu.Unlock()
		task.addLog(fmt.Sprintf("完成: 总计 %d, 成功 %d, 失败 %d", task.Total, task.Completed, task.Failed))

		if cfg.NotifyWebhook != "" {
			s.sendParseNotification(task, cfg.NotifyWebhook, cfg.NotifyRoom)
		}
	}()

	currentBatchSize := cfg.BatchSize
	currentDelay := time.Duration(cfg.InitialDelay) * time.Second
	consecutiveSuccess := 0
	var retryQueue []FailedDoc

	for _, item := range items {
		if len(item.DocumentIDs) == 0 {
			continue
		}
		task.addLog(fmt.Sprintf("开始处理 dataset %s (%d 个文档)", item.DatasetID, len(item.DocumentIDs)))

		for i := 0; i < len(item.DocumentIDs); i += currentBatchSize {
			end := i + currentBatchSize
			if end > len(item.DocumentIDs) {
				end = len(item.DocumentIDs)
			}
			batch := item.DocumentIDs[i:end]
			batchNum := i/currentBatchSize + 1

			task.addLog(fmt.Sprintf("提交批次 %d (%d 个文档, batch_size=%d)", batchNum, len(batch), currentBatchSize))

			// Submit batch
			_, err := s.RunParsing(item.DatasetID, batch)
			if err != nil {
				task.addLog(fmt.Sprintf("批次 %d 提交失败: %v", batchNum, err))

				if isRateLimitError(err.Error()) {
					currentBatchSize = maxInt(currentBatchSize/2, 1)
					currentDelay = currentDelay * 2
					consecutiveSuccess = 0
					task.addLog(fmt.Sprintf("检测到速率限制，调整: batch_size=%d, delay=%v", currentBatchSize, currentDelay))
				}

				for _, docID := range batch {
					retryQueue = append(retryQueue, FailedDoc{
						DatasetID: item.DatasetID, DocumentID: docID,
						Error: err.Error(),
					})
				}
				task.mu.Lock()
				task.Pending -= len(batch)
				task.Failed += len(batch)
				task.BatchSize = currentBatchSize
				task.mu.Unlock()

				time.Sleep(currentDelay)
				continue
			}

			// Poll for batch completion
			batchSuccess, batchFailed := s.pollBatchStatus(task, item.DatasetID, batch, cfg)

			task.mu.Lock()
			task.Completed += batchSuccess
			task.Failed += len(batchFailed)
			task.Pending -= len(batch)
			task.BatchSize = currentBatchSize
			task.mu.Unlock()

			retryQueue = append(retryQueue, batchFailed...)

			// Adaptive rate adjustment
			if len(batchFailed) == 0 {
				consecutiveSuccess++
				if consecutiveSuccess >= 2 && currentBatchSize < cfg.BatchSize {
					currentBatchSize = minInt(currentBatchSize+1, cfg.BatchSize)
					currentDelay = time.Duration(cfg.InitialDelay) * time.Second
					task.addLog(fmt.Sprintf("连续成功，恢复: batch_size=%d", currentBatchSize))
				}
			} else {
				hasRateLimit := false
				for _, fd := range batchFailed {
					if isRateLimitError(fd.Error) {
						hasRateLimit = true
						break
					}
				}
				if hasRateLimit {
					currentBatchSize = maxInt(currentBatchSize/2, 1)
					currentDelay = currentDelay * 2
					consecutiveSuccess = 0
					task.addLog(fmt.Sprintf("检测到速率限制，调整: batch_size=%d, delay=%v", currentBatchSize, currentDelay))
				} else {
					consecutiveSuccess = 0
				}
			}

			if end < len(item.DocumentIDs) {
				time.Sleep(currentDelay)
			}
		}
	}

	// Retry queue
	if len(retryQueue) > 0 {
		task.addLog(fmt.Sprintf("开始重试队列: %d 个文档", len(retryQueue)))
		retryDelay := currentDelay * 2
		if retryDelay < 30*time.Second {
			retryDelay = 30 * time.Second
		}

		for round := 1; round <= cfg.MaxRetries; round++ {
			if len(retryQueue) == 0 {
				break
			}
			task.addLog(fmt.Sprintf("重试第 %d 轮 (%d 个文档)", round, len(retryQueue)))
			var stillFailed []FailedDoc

			for _, fd := range retryQueue {
				if fd.Retries >= cfg.MaxRetries {
					stillFailed = append(stillFailed, fd)
					continue
				}

				time.Sleep(retryDelay)
				_, err := s.RunParsing(fd.DatasetID, []string{fd.DocumentID})
				if err != nil {
					fd.Retries++
					fd.Error = err.Error()
					stillFailed = append(stillFailed, fd)
					continue
				}

				status, errMsg := s.pollSingleDocStatus(fd.DatasetID, fd.DocumentID, cfg)
				if status == "parsed" {
					task.mu.Lock()
					task.Completed++
					task.Failed--
					task.mu.Unlock()
					shortID := fd.DocumentID
					if len(shortID) > 8 {
						shortID = shortID[:8]
					}
					task.addLog(fmt.Sprintf("重试成功: %s", shortID))
				} else {
					fd.Retries++
					fd.Error = errMsg
					stillFailed = append(stillFailed, fd)
				}
			}
			retryQueue = stillFailed
		}

		if len(retryQueue) > 0 {
			task.mu.Lock()
			task.FailedDocs = retryQueue
			task.mu.Unlock()
			task.addLog(fmt.Sprintf("重试完成，仍有 %d 个文档失败", len(retryQueue)))
		}
	}
}

func (s *RagFlowService) pollBatchStatus(task *ParseTask, datasetID string, docIDs []string, cfg ParseQueueConfig) (int, []FailedDoc) {
	timeout := time.Duration(cfg.PollTimeout) * time.Second
	interval := time.Duration(cfg.PollInterval) * time.Second
	deadline := time.Now().Add(timeout)

	pending := make(map[string]bool, len(docIDs))
	for _, id := range docIDs {
		pending[id] = true
	}

	successCount := 0
	var failedDocs []FailedDoc

	for time.Now().Before(deadline) && len(pending) > 0 {
		time.Sleep(interval)

		for docID := range pending {
			result, err := s.GetParsingStatus(datasetID, docID)
			if err != nil {
				continue // network error, keep polling
			}

			status := extractDocStatus(result)
			switch status {
			case "parsed":
				delete(pending, docID)
				successCount++
			case "error":
				delete(pending, docID)
				failedDocs = append(failedDocs, FailedDoc{
					DatasetID:  datasetID,
					DocumentID: docID,
					Error:      extractDocError(result),
				})
			}
		}
	}

	// Treat remaining as timeout
	for docID := range pending {
		failedDocs = append(failedDocs, FailedDoc{
			DatasetID:  datasetID,
			DocumentID: docID,
			Error:      "polling timeout",
		})
	}

	return successCount, failedDocs
}

func (s *RagFlowService) pollSingleDocStatus(datasetID, documentID string, cfg ParseQueueConfig) (string, string) {
	timeout := time.Duration(cfg.PollTimeout) * time.Second
	interval := time.Duration(cfg.PollInterval) * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		result, err := s.GetParsingStatus(datasetID, documentID)
		if err != nil {
			return "error", err.Error()
		}

		status := extractDocStatus(result)
		switch status {
		case "parsed":
			return "parsed", ""
		case "error":
			return "error", extractDocError(result)
		}
		time.Sleep(interval)
	}
	return "error", "polling timeout"
}

func extractDocStatus(result map[string]interface{}) string {
	data, ok := result["data"]
	if !ok {
		return "unknown"
	}
	d, ok := data.(map[string]interface{})
	if !ok {
		return "unknown"
	}
	docs, ok := d["docs"].([]interface{})
	if !ok || len(docs) == 0 {
		return "unknown"
	}
	doc, ok := docs[0].(map[string]interface{})
	if !ok {
		return "unknown"
	}

	// Try string status field
	if status, ok := doc["status"].(string); ok {
		return normalizeDocStatus(status)
	}
	// Try numeric status
	if status, ok := doc["status"].(float64); ok {
		switch int(status) {
		case 0:
			return "unstart"
		case 1:
			return "parsing"
		case 2:
			return "parsed"
		case 3:
			return "error"
		}
	}
	// Try "run" field
	if run, ok := doc["run"].(string); ok {
		return normalizeDocStatus(run)
	}
	return "unknown"
}

func normalizeDocStatus(s string) string {
	switch strings.ToLower(s) {
	case "0", "unstart":
		return "unstart"
	case "1", "parsing", "running":
		return "parsing"
	case "2", "parsed", "done", "success":
		return "parsed"
	case "3", "error", "fail", "failed":
		return "error"
	default:
		return s
	}
}

func extractDocError(result map[string]interface{}) string {
	data, ok := result["data"]
	if !ok {
		return "unknown error"
	}
	d, ok := data.(map[string]interface{})
	if !ok {
		return "unknown error"
	}
	docs, ok := d["docs"].([]interface{})
	if !ok || len(docs) == 0 {
		return "unknown error"
	}
	doc, ok := docs[0].(map[string]interface{})
	if !ok {
		return "unknown error"
	}

	for _, field := range []string{"progress_msg", "process_msg", "message"} {
		if msg, ok := doc[field].(string); ok && msg != "" {
			return msg
		}
	}
	return "parse error"
}

func isRateLimitError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	for _, p := range []string{"429", "rate limit", "rate_limit", "quota", "too many requests", "throttl"} {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (s *RagFlowService) sendParseNotification(task *ParseTask, webhookURL, room string) {
	snap := task.snapshot()

	duration := ""
	if snap.CompletedAt != nil {
		d := snap.CompletedAt.Sub(snap.StartedAt)
		if d.Minutes() >= 1 {
			duration = fmt.Sprintf("%.1f 分钟", d.Minutes())
		} else {
			duration = fmt.Sprintf("%d 秒", int(d.Seconds()))
		}
	}

	message := fmt.Sprintf("文档解析完成\n总计: %d | 成功: %d | 失败: %d\n耗时: %s",
		snap.Total, snap.Completed, snap.Failed, duration)
	html := fmt.Sprintf("<b>文档解析完成</b><br>总计: %d | 成功: %d | 失败: %d<br>耗时: %s",
		snap.Total, snap.Completed, snap.Failed, duration)

	if len(snap.FailedDocs) > 0 {
		message += fmt.Sprintf("\n\n失败文档 (%d):", len(snap.FailedDocs))
		html += fmt.Sprintf("<br><br><b>失败文档 (%d):</b><ul>", len(snap.FailedDocs))
		limit := len(snap.FailedDocs)
		if limit > 10 {
			limit = 10
		}
		for _, fd := range snap.FailedDocs[:limit] {
			shortID := fd.DocumentID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			message += fmt.Sprintf("\n- %s: %s (重试 %d 次)", shortID, fd.Error, fd.Retries)
			html += fmt.Sprintf("<li>%s: %s (重试 %d 次)</li>", shortID, fd.Error, fd.Retries)
		}
		if len(snap.FailedDocs) > 10 {
			message += fmt.Sprintf("\n... 等 %d 个", len(snap.FailedDocs)-10)
			html += fmt.Sprintf("<li>... 等 %d 个</li>", len(snap.FailedDocs)-10)
		}
		html += "</ul>"
	}

	payload := map[string]interface{}{
		"room":    room,
		"message": message,
		"html":    html,
	}
	if _, err := s.notifyPost(webhookURL, payload); err != nil {
		log.Printf("warn: [parse-queue %s] notification failed: %v", task.ID, err)
	}
}

func (s *RagFlowService) notifyPost(webhookURL string, payload map[string]interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	return result, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
