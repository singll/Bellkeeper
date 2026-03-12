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

type batchResolution struct {
	Succeeded    []string
	FinalFailed  []FailedDoc
	Suspected    []string
	HadRecovery  bool
}

type docStatusSnapshot struct {
	Status   string
	Error    string
	Progress string
}

// ParseQueueItem represents a dataset and its documents to parse.
type ParseQueueItem struct {
	DatasetID   string   `json:"dataset_id"`
	DocumentIDs []string `json:"document_ids"`
}

// ParseQueueConfig controls parsing queue behavior.
type ParseQueueConfig struct {
	BatchSize                  int    `json:"batch_size"`                     // initial batch size (default 3)
	MaxRetries                 int    `json:"max_retries"`                    // backward-compatible fallback for recovery attempts
	MaxRecoveryAttempts        int    `json:"max_recovery_attempts"`          // per-doc recovery attempts
	InitialDelay               int    `json:"initial_delay"`                  // seconds between batches (default 15)
	PollInterval               int    `json:"poll_interval"`                  // seconds between status polls (default 10)
	PollTimeout                int    `json:"poll_timeout"`                   // backward-compatible fallback for soft timeout
	SoftTimeout                int    `json:"soft_timeout"`                   // max seconds for normal polling window
	HardTimeout                int    `json:"hard_timeout"`                   // max seconds before terminal timeout
	StallWindow                int    `json:"stall_window"`                   // progress stall window before treating as stuck
	DegradeToSingleOnTimeout   bool   `json:"degrade_to_single_on_timeout"`   // reduce next batches to single-doc after recovery
	NotifyWebhook              string `json:"notify_webhook"`                 // B01 webhook URL for completion notification
	NotifyRoom                 string `json:"notify_room"`                    // Matrix room for notification
}

// ParseDocState tracks runtime state for a single document.
type ParseDocState struct {
	DatasetID         string     `json:"dataset_id"`
	DocumentID        string     `json:"document_id"`
	CurrentStatus     string     `json:"current_status"`
	Stage             string     `json:"stage"`
	SubmittedAt       *time.Time `json:"submitted_at,omitempty"`
	LastProgressAt    *time.Time `json:"last_progress_at,omitempty"`
	LastStateChangeAt *time.Time `json:"last_state_change_at,omitempty"`
	RecoveryAttempts  int        `json:"recovery_attempts"`
	LastError         string     `json:"last_error,omitempty"`
	lastProgressText  string     `json:"-"`
}

// ParseTask tracks the progress of a smart parsing queue.
type ParseTask struct {
	mu                 sync.Mutex                 `json:"-"`
	ID                 string                     `json:"id"`
	Status             string                     `json:"status"` // running / recovering / completed
	Total              int                        `json:"total"`
	Completed          int                        `json:"completed"`
	Failed             int                        `json:"failed"`
	Pending            int                        `json:"pending"`
	BatchSize          int                        `json:"batch_size"`
	RunningCount       int                        `json:"running_count"`
	RecoveringCount    int                        `json:"recovering_count"`
	SucceededCount     int                        `json:"succeeded_count"`
	FinalFailedCount   int                        `json:"final_failed_count"`
	CurrentDatasetID   string                     `json:"current_dataset_id,omitempty"`
	CurrentBatchIndex  int                        `json:"current_batch_index,omitempty"`
	CurrentStage       string                     `json:"current_stage,omitempty"`
	ResultStatus       string                     `json:"result_status,omitempty"`
	FailedDocs         []FailedDoc                `json:"failed_docs,omitempty"`
	SuspectedStuckDocs []string                   `json:"suspected_stuck_docs,omitempty"`
	DocStates          []ParseDocState            `json:"doc_states,omitempty"`
	StartedAt          time.Time                  `json:"started_at"`
	LastProgressAt     *time.Time                 `json:"last_progress_at,omitempty"`
	CompletedAt        *time.Time                 `json:"completed_at,omitempty"`
	Log                []string                   `json:"log,omitempty"`
	docStates          map[string]*ParseDocState  `json:"-"`
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
	id := t.ID
	t.mu.Unlock()

	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)

	t.mu.Lock()
	t.Log = append(t.Log, entry)
	t.mu.Unlock()

	log.Printf("info: [parse-queue %s] %s", id, msg)
}

func (t *ParseTask) snapshot() ParseTask {
	t.mu.Lock()
	defer t.mu.Unlock()
	failedDocs := make([]FailedDoc, len(t.FailedDocs))
	copy(failedDocs, t.FailedDocs)
	logs := make([]string, len(t.Log))
	copy(logs, t.Log)
	suspected := append([]string(nil), t.SuspectedStuckDocs...)
	docStates := make([]ParseDocState, 0, len(t.docStates))
	for _, state := range t.docStates {
		copied := *state
		docStates = append(docStates, copied)
	}
	return ParseTask{
		ID:                 t.ID,
		Status:             t.Status,
		Total:              t.Total,
		Completed:          t.Completed,
		Failed:             t.Failed,
		Pending:            t.Pending,
		BatchSize:          t.BatchSize,
		RunningCount:       t.RunningCount,
		RecoveringCount:    t.RecoveringCount,
		SucceededCount:     t.SucceededCount,
		FinalFailedCount:   t.FinalFailedCount,
		CurrentDatasetID:   t.CurrentDatasetID,
		CurrentBatchIndex:  t.CurrentBatchIndex,
		CurrentStage:       t.CurrentStage,
		ResultStatus:       t.ResultStatus,
		FailedDocs:         failedDocs,
		SuspectedStuckDocs: suspected,
		DocStates:          docStates,
		StartedAt:          t.StartedAt,
		LastProgressAt:     t.LastProgressAt,
		CompletedAt:        t.CompletedAt,
		Log:                logs,
	}
}

func (t *ParseTask) initDocStates(items []ParseQueueItem) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.docStates == nil {
		t.docStates = make(map[string]*ParseDocState)
	}
	for _, item := range items {
		for _, docID := range item.DocumentIDs {
			key := taskDocKey(item.DatasetID, docID)
			if _, exists := t.docStates[key]; exists {
				continue
			}
			t.docStates[key] = &ParseDocState{
				DatasetID:     item.DatasetID,
				DocumentID:    docID,
				CurrentStatus: "queued",
				Stage:         "queued",
			}
		}
	}
	t.refreshCountsLocked()
}

func (t *ParseTask) markBatch(datasetID string, batchIndex int, batchSize int, stage string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.CurrentDatasetID = datasetID
	t.CurrentBatchIndex = batchIndex
	t.BatchSize = batchSize
	t.CurrentStage = stage
	if stage == "recovering" {
		t.Status = "recovering"
	} else if t.Status != "completed" {
		t.Status = "running"
	}
}

func (t *ParseTask) markSubmitted(datasetID string, docIDs []string) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, docID := range docIDs {
		state := t.ensureDocStateLocked(datasetID, docID)
		state.SubmittedAt = &now
		state.LastStateChangeAt = &now
		if state.LastProgressAt == nil {
			state.LastProgressAt = &now
		}
		state.CurrentStatus = "submitted"
		state.Stage = "running"
		state.LastError = ""
	}
	t.LastProgressAt = &now
	t.refreshCountsLocked()
}

func (t *ParseTask) observeDoc(datasetID, docID string, snap docStatusSnapshot, stage string) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.ensureDocStateLocked(datasetID, docID)
	if state.CurrentStatus != snap.Status {
		state.CurrentStatus = snap.Status
		state.LastStateChangeAt = &now
	}
	if stage != "" && state.Stage != "succeeded" && state.Stage != "final_failed" {
		state.Stage = stage
	}
	if snap.Error != "" {
		state.LastError = snap.Error
	}
	if snap.Progress != "" && snap.Progress != state.lastProgressText {
		state.lastProgressText = snap.Progress
		state.LastProgressAt = &now
		t.LastProgressAt = &now
	}
	if state.LastProgressAt == nil {
		state.LastProgressAt = &now
	}
	t.refreshCountsLocked()
}

func (t *ParseTask) markRecovering(datasetID, docID string, errMsg string) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.ensureDocStateLocked(datasetID, docID)
	state.Stage = "recovering"
	state.LastError = errMsg
	state.LastStateChangeAt = &now
	if state.LastProgressAt == nil {
		state.LastProgressAt = &now
	}
	t.Status = "recovering"
	if !containsDoc(t.SuspectedStuckDocs, docID) {
		t.SuspectedStuckDocs = append(t.SuspectedStuckDocs, docID)
	}
	t.refreshCountsLocked()
}

func (t *ParseTask) recordRecoveryAttempt(datasetID, docID string) int {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.ensureDocStateLocked(datasetID, docID)
	state.RecoveryAttempts++
	state.Stage = "recovering"
	state.LastStateChangeAt = &now
	if state.LastProgressAt == nil {
		state.LastProgressAt = &now
	}
	t.Status = "recovering"
	t.refreshCountsLocked()
	return state.RecoveryAttempts
}

func (t *ParseTask) markResubmitted(datasetID, docID string) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.ensureDocStateLocked(datasetID, docID)
	state.Stage = "running"
	state.CurrentStatus = "resubmitted"
	state.SubmittedAt = &now
	state.LastStateChangeAt = &now
	state.LastProgressAt = &now
	t.LastProgressAt = &now
	t.SuspectedStuckDocs = removeDoc(t.SuspectedStuckDocs, docID)
	t.refreshCountsLocked()
}

func (t *ParseTask) finalizeDoc(datasetID, docID, finalStatus, errMsg string, retries int) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.ensureDocStateLocked(datasetID, docID)
	state.CurrentStatus = finalStatus
	state.LastError = errMsg
	state.LastStateChangeAt = &now
	state.LastProgressAt = &now
	if finalStatus == "parsed" {
		state.Stage = "succeeded"
		t.FailedDocs = removeFailedDoc(t.FailedDocs, datasetID, docID)
	} else {
		state.Stage = "final_failed"
		t.FailedDocs = upsertFailedDoc(t.FailedDocs, FailedDoc{
			DatasetID:  datasetID,
			DocumentID: docID,
			Error:      errMsg,
			Retries:    retries,
		})
	}
	t.SuspectedStuckDocs = removeDoc(t.SuspectedStuckDocs, docID)
	t.LastProgressAt = &now
	t.refreshCountsLocked()
}

func (t *ParseTask) ensureDocStateLocked(datasetID, docID string) *ParseDocState {
	if t.docStates == nil {
		t.docStates = make(map[string]*ParseDocState)
	}
	key := taskDocKey(datasetID, docID)
	if state, ok := t.docStates[key]; ok {
		return state
	}
	state := &ParseDocState{
		DatasetID:     datasetID,
		DocumentID:    docID,
		CurrentStatus: "queued",
		Stage:         "queued",
	}
	t.docStates[key] = state
	return state
}

func (t *ParseTask) refreshCountsLocked() {
	running := 0
	recovering := 0
	succeeded := 0
	finalFailed := 0
	for _, state := range t.docStates {
		switch state.Stage {
		case "succeeded":
			succeeded++
		case "final_failed":
			finalFailed++
		case "recovering":
			recovering++
			running++
		default:
			running++
		}
	}
	t.RunningCount = running
	t.RecoveringCount = recovering
	t.SucceededCount = succeeded
	t.FinalFailedCount = finalFailed
	t.Completed = succeeded
	t.Failed = finalFailed
	t.Pending = t.Total - succeeded - finalFailed
	if t.Pending < 0 {
		t.Pending = 0
	}
}

// RunParsingQueue starts a smart parsing queue in the background.
// Returns the task ID for progress tracking.
func (s *RagFlowService) RunParsingQueue(items []ParseQueueItem, cfg ParseQueueConfig) string {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 3
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 15
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10
	}
	if cfg.SoftTimeout <= 0 {
		cfg.SoftTimeout = cfg.PollTimeout
	}
	if cfg.SoftTimeout <= 0 {
		cfg.SoftTimeout = 300
	}
	if cfg.HardTimeout <= 0 {
		cfg.HardTimeout = maxInt(cfg.SoftTimeout*2, cfg.SoftTimeout+300)
	}
	if cfg.StallWindow <= 0 {
		cfg.StallWindow = maxInt(cfg.PollInterval*3, 30)
	}
	if cfg.MaxRecoveryAttempts <= 0 {
		cfg.MaxRecoveryAttempts = cfg.MaxRetries
	}
	if cfg.MaxRecoveryAttempts <= 0 {
		cfg.MaxRecoveryAttempts = 3
	}

	totalDocs := 0
	for _, item := range items {
		totalDocs += len(item.DocumentIDs)
	}

	taskID := fmt.Sprintf("parse-%d", time.Now().UnixMilli())
	task := &ParseTask{
		ID:           taskID,
		Status:       "running",
		Total:        totalDocs,
		Pending:      totalDocs,
		BatchSize:    cfg.BatchSize,
		StartedAt:    time.Now(),
		CurrentStage: "queued",
		docStates:    make(map[string]*ParseDocState),
	}
	task.initDocStates(items)
	parseTasks.Store(taskID, task)

	if s.activityLog != nil {
		s.activityLog.LogActivity(LogActivityParams{
			Module:  "ragflow_parse",
			Action:  "task_start",
			Status:  "info",
			Summary: fmt.Sprintf("解析任务开始: %s (%d docs)", taskID, totalDocs),
			RefID:   taskID,
		})
	}

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

// ListParseTasks returns snapshots of all parse tasks currently tracked in memory.
func (s *RagFlowService) ListParseTasks() []ParseTask {
	var out []ParseTask
	parseTasks.Range(func(_, v any) bool {
		task := v.(*ParseTask)
		snap := task.snapshot()
		out = append(out, snap)
		return true
	})
	return out
}

func (s *RagFlowService) executeParsingQueue(task *ParseTask, items []ParseQueueItem, cfg ParseQueueConfig) {
	defer func() {
		now := time.Now()
		task.mu.Lock()
		task.Status = "completed"
		task.CurrentStage = "completed"
		task.CompletedAt = &now
		task.ResultStatus = deriveResultStatus(task.Completed, task.Failed, task.Total)
		task.refreshCountsLocked()
		resultStatus := task.ResultStatus
		completed := task.Completed
		failed := task.Failed
		total := task.Total
		task.mu.Unlock()
		task.addLog(fmt.Sprintf("完成: 总计 %d, 成功 %d, 失败 %d", total, completed, failed))

		if s.activityLog != nil {
			status := "success"
			if resultStatus != "success" {
				status = "error"
			}
			s.activityLog.LogActivity(LogActivityParams{
				Module: "ragflow_parse", Action: "task_complete", Status: status,
				Summary: fmt.Sprintf("解析任务完成: %s (%s) 成功 %d / 失败 %d / 总计 %d", task.ID, resultStatus, completed, failed, total),
				RefID: task.ID,
				DurationMs: int(time.Since(task.StartedAt).Milliseconds()),
			})
		}

		if cfg.NotifyWebhook != "" {
			s.sendParseNotification(task, cfg.NotifyWebhook, cfg.NotifyRoom)
		}
	}()

	currentBatchSize := cfg.BatchSize
	currentDelay := time.Duration(cfg.InitialDelay) * time.Second
	consecutiveStableBatches := 0

	for _, item := range items {
		if len(item.DocumentIDs) == 0 {
			continue
		}
		if s.activityLog != nil {
			s.activityLog.LogActivity(LogActivityParams{
				Module:  "ragflow_parse",
				Action:  "dataset_start",
				Status:  "info",
				Summary: fmt.Sprintf("开始处理 dataset %s (%d docs)", item.DatasetID, len(item.DocumentIDs)),
				RefID:   task.ID,
				Detail: map[string]any{"dataset_id": item.DatasetID, "docs": len(item.DocumentIDs)},
			})
		}
		task.addLog(fmt.Sprintf("开始处理 dataset %s (%d 个文档)", item.DatasetID, len(item.DocumentIDs)))

		batchIndex := 0
		for i := 0; i < len(item.DocumentIDs); i += currentBatchSize {
			end := i + currentBatchSize
			if end > len(item.DocumentIDs) {
				end = len(item.DocumentIDs)
			}
			batchIndex++
			batch := item.DocumentIDs[i:end]

			task.markBatch(item.DatasetID, batchIndex, currentBatchSize, "submitting")
			task.addLog(fmt.Sprintf("提交批次 %d (%d 个文档, batch_size=%d)", batchIndex, len(batch), currentBatchSize))
			if s.activityLog != nil {
				s.activityLog.LogActivity(LogActivityParams{
					Module:  "ragflow_parse",
					Action:  "batch_submit",
					Status:  "info",
					Summary: fmt.Sprintf("提交批次 %d (%d docs) dataset=%s", batchIndex, len(batch), item.DatasetID),
					RefID:   task.ID,
					Detail: map[string]any{"dataset_id": item.DatasetID, "batch_index": batchIndex, "batch_size": currentBatchSize, "docs": len(batch)},
				})
			}
			_, err := s.RunParsing(item.DatasetID, batch)
			if err != nil {
				task.addLog(fmt.Sprintf("批次 %d 提交失败: %v", batchIndex, err))
				if s.activityLog != nil {
					s.activityLog.LogActivity(LogActivityParams{
						Module:  "ragflow_parse",
						Action:  "batch_submit",
						Status:  "error",
						Summary: fmt.Sprintf("批次 %d 提交失败: %v", batchIndex, err),
						RefID:   task.ID,
						Detail: map[string]any{"dataset_id": item.DatasetID, "batch_index": batchIndex, "error": err.Error()},
					})
				}
				if isRateLimitError(err.Error()) {
					currentBatchSize = maxInt(currentBatchSize/2, 1)
					currentDelay = currentDelay * 2
					consecutiveStableBatches = 0
					task.addLog(fmt.Sprintf("检测到速率限制，调整: batch_size=%d, delay=%v", currentBatchSize, currentDelay))
				}
				for _, docID := range batch {
					task.finalizeDoc(item.DatasetID, docID, "error", err.Error(), 0)
				}
				if end < len(item.DocumentIDs) {
					time.Sleep(currentDelay)
				}
				continue
			}

			task.markSubmitted(item.DatasetID, batch)
			task.markBatch(item.DatasetID, batchIndex, currentBatchSize, "polling")
			resolution := s.resolveBatch(task, item.DatasetID, batch, cfg)

			if resolution.HadRecovery && cfg.DegradeToSingleOnTimeout {
				currentBatchSize = 1
				consecutiveStableBatches = 0
				task.addLog("检测到恢复流程，后续批次降级为单文档解析")
			} else if !resolution.HadRecovery {
				consecutiveStableBatches++
				if consecutiveStableBatches >= 2 && currentBatchSize < cfg.BatchSize {
					currentBatchSize = minInt(currentBatchSize+1, cfg.BatchSize)
					task.addLog(fmt.Sprintf("连续稳定，恢复批次大小为 %d", currentBatchSize))
				}
			} else {
				consecutiveStableBatches = 0
			}

			if end < len(item.DocumentIDs) {
				time.Sleep(currentDelay)
			}
		}
	}
}

func (s *RagFlowService) resolveBatch(task *ParseTask, datasetID string, docIDs []string, cfg ParseQueueConfig) batchResolution {
	resolution := s.waitBatchUntilSettled(task, datasetID, docIDs, cfg)
	for _, docID := range resolution.Succeeded {
		task.finalizeDoc(datasetID, docID, "parsed", "", 0)
	}
	for _, failed := range resolution.FinalFailed {
		task.finalizeDoc(datasetID, failed.DocumentID, "error", failed.Error, failed.Retries)
	}
	if len(resolution.Suspected) == 0 {
		return resolution
	}

	resolution.HadRecovery = true
	snap := task.snapshot()
	task.markBatch(datasetID, snap.CurrentBatchIndex, snap.BatchSize, "recovering")
	for _, docID := range resolution.Suspected {
		failed := s.resolveSingleDocTerminalState(task, datasetID, docID, cfg)
		if failed == nil {
			task.finalizeDoc(datasetID, docID, "parsed", "", 0)
			continue
		}
		task.finalizeDoc(datasetID, docID, "error", failed.Error, failed.Retries)
	}
	return resolution
}

func (s *RagFlowService) waitBatchUntilSettled(task *ParseTask, datasetID string, docIDs []string, cfg ParseQueueConfig) batchResolution {
	softDeadline := time.Now().Add(time.Duration(cfg.SoftTimeout) * time.Second)
	interval := time.Duration(cfg.PollInterval) * time.Second
	stallWindow := time.Duration(cfg.StallWindow) * time.Second
	pending := make(map[string]bool, len(docIDs))
	for _, docID := range docIDs {
		pending[docID] = true
	}
	resolution := batchResolution{}

	for len(pending) > 0 && time.Now().Before(softDeadline) {
		time.Sleep(interval)
		for docID := range pending {
			result, err := s.GetParsingStatus(datasetID, docID)
			if err != nil {
				continue
			}
			snap := getDocStatusSnapshot(result)
			if shouldEnterRecovery(task, datasetID, docID, snap, stallWindow) {
				delete(pending, docID)
				resolution.Suspected = append(resolution.Suspected, docID)
				task.markRecovering(datasetID, docID, "stall window exceeded")
				task.addLog(fmt.Sprintf("文档 %s 疑似卡住，进入恢复流程", shortDocID(docID)))
				continue
			}
			task.observeDoc(datasetID, docID, snap, "running")
			switch snap.Status {
			case "parsed":
				delete(pending, docID)
				resolution.Succeeded = append(resolution.Succeeded, docID)
			case "error":
				delete(pending, docID)
				resolution.FinalFailed = append(resolution.FinalFailed, FailedDoc{
					DatasetID:  datasetID,
					DocumentID: docID,
					Error:      firstNonEmpty(snap.Error, "parse error"),
				})
			}
		}
	}

	for docID := range pending {
		resolution.Suspected = append(resolution.Suspected, docID)
		task.markRecovering(datasetID, docID, "soft timeout reached")
		task.addLog(fmt.Sprintf("文档 %s 超过 soft timeout，进入恢复流程", shortDocID(docID)))
	}
	if len(resolution.Suspected) > 0 {
		resolution.HadRecovery = true
	}
	return resolution
}

func shouldEnterRecovery(task *ParseTask, datasetID, docID string, snap docStatusSnapshot, stallWindow time.Duration) bool {
	if snap.Status == "parsed" || snap.Status == "error" {
		return false
	}
	if stallWindow <= 0 {
		return false
	}
	task.mu.Lock()
	defer task.mu.Unlock()
	state, ok := task.docStates[taskDocKey(datasetID, docID)]
	if !ok || state.LastProgressAt == nil {
		return false
	}
	return time.Since(*state.LastProgressAt) >= stallWindow
}

func (s *RagFlowService) resolveSingleDocTerminalState(task *ParseTask, datasetID, documentID string, cfg ParseQueueConfig) *FailedDoc {
	hardDeadline := time.Now().Add(time.Duration(cfg.HardTimeout) * time.Second)
	for attempt := 1; attempt <= cfg.MaxRecoveryAttempts; attempt++ {
		if time.Now().After(hardDeadline) {
			task.addLog(fmt.Sprintf("文档 %s 超过 hard timeout，恢复失败", shortDocID(documentID)))
			return &FailedDoc{DatasetID: datasetID, DocumentID: documentID, Error: "hard timeout after recovery", Retries: attempt - 1}
		}

		recordedAttempt := task.recordRecoveryAttempt(datasetID, documentID)
		task.markRecovering(datasetID, documentID, fmt.Sprintf("recovery attempt %d", recordedAttempt))
		task.addLog(fmt.Sprintf("开始恢复文档 %s (第 %d 次)", shortDocID(documentID), recordedAttempt))

		status, errMsg := s.recheckDocStatus(task, datasetID, documentID, cfg, 3)
		switch status {
		case "parsed":
			task.addLog(fmt.Sprintf("文档 %s 复查后确认已完成", shortDocID(documentID)))
			return nil
		case "error":
			return &FailedDoc{DatasetID: datasetID, DocumentID: documentID, Error: errMsg, Retries: recordedAttempt - 1}
		}

		task.addLog(fmt.Sprintf("停止文档 %s 的解析任务", shortDocID(documentID)))
		if _, err := s.StopParsing(datasetID, []string{documentID}); err != nil {
			task.addLog(fmt.Sprintf("停止文档 %s 失败: %v", shortDocID(documentID), err))
			if attempt == cfg.MaxRecoveryAttempts {
				return &FailedDoc{DatasetID: datasetID, DocumentID: documentID, Error: fmt.Sprintf("stop parsing failed: %v", err), Retries: recordedAttempt}
			}
			continue
		}

		status, errMsg = s.recheckDocStatus(task, datasetID, documentID, cfg, 2)
		switch status {
		case "parsed":
			task.addLog(fmt.Sprintf("文档 %s 在 stop 后确认已完成", shortDocID(documentID)))
			return nil
		case "error":
			return &FailedDoc{DatasetID: datasetID, DocumentID: documentID, Error: errMsg, Retries: recordedAttempt}
		}

		task.addLog(fmt.Sprintf("重新提交文档 %s 进行单文档解析", shortDocID(documentID)))
		if _, err := s.RunParsing(datasetID, []string{documentID}); err != nil {
			task.addLog(fmt.Sprintf("重提文档 %s 失败: %v", shortDocID(documentID), err))
			if attempt == cfg.MaxRecoveryAttempts {
				return &FailedDoc{DatasetID: datasetID, DocumentID: documentID, Error: fmt.Sprintf("resubmit failed: %v", err), Retries: recordedAttempt}
			}
			continue
		}
		task.markResubmitted(datasetID, documentID)

		status, errMsg = s.waitForSingleDocTerminalState(task, datasetID, documentID, cfg, hardDeadline)
		switch status {
		case "parsed":
			task.addLog(fmt.Sprintf("文档 %s 恢复成功", shortDocID(documentID)))
			return nil
		case "error":
			return &FailedDoc{DatasetID: datasetID, DocumentID: documentID, Error: errMsg, Retries: recordedAttempt}
		}
	}
	return &FailedDoc{DatasetID: datasetID, DocumentID: documentID, Error: "recovery exhausted", Retries: cfg.MaxRecoveryAttempts}
}

func (s *RagFlowService) recheckDocStatus(task *ParseTask, datasetID, documentID string, cfg ParseQueueConfig, checks int) (string, string) {
	interval := time.Duration(maxInt(cfg.PollInterval/2, 2)) * time.Second
	for i := 0; i < checks; i++ {
		if i > 0 {
			time.Sleep(interval)
		}
		result, err := s.GetParsingStatus(datasetID, documentID)
		if err != nil {
			continue
		}
		snap := getDocStatusSnapshot(result)
		task.observeDoc(datasetID, documentID, snap, "recovering")
		switch snap.Status {
		case "parsed":
			return "parsed", ""
		case "error":
			return "error", firstNonEmpty(snap.Error, "parse error")
		}
	}
	return "running", ""
}

func (s *RagFlowService) waitForSingleDocTerminalState(task *ParseTask, datasetID, documentID string, cfg ParseQueueConfig, hardDeadline time.Time) (string, string) {
	interval := time.Duration(cfg.PollInterval) * time.Second
	softDeadline := time.Now().Add(time.Duration(cfg.SoftTimeout) * time.Second)
	for time.Now().Before(softDeadline) && time.Now().Before(hardDeadline) {
		result, err := s.GetParsingStatus(datasetID, documentID)
		if err == nil {
			snap := getDocStatusSnapshot(result)
			task.observeDoc(datasetID, documentID, snap, "running")
			switch snap.Status {
			case "parsed":
				return "parsed", ""
			case "error":
				return "error", firstNonEmpty(snap.Error, "parse error")
			}
		}
		time.Sleep(interval)
	}
	if time.Now().After(hardDeadline) {
		return "error", "hard timeout after recovery"
	}
	return "running", "soft timeout after resubmit"
}

func getDocStatusSnapshot(result map[string]interface{}) docStatusSnapshot {
	return docStatusSnapshot{
		Status:   extractDocStatus(result),
		Error:    extractDocError(result),
		Progress: extractDocProgress(result),
	}
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

	if status, ok := doc["status"].(string); ok {
		return normalizeDocStatus(status)
	}
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
		return strings.ToLower(s)
	}
}

func extractDocProgress(result map[string]interface{}) string {
	data, ok := result["data"]
	if !ok {
		return ""
	}
	d, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}
	docs, ok := d["docs"].([]interface{})
	if !ok || len(docs) == 0 {
		return ""
	}
	doc, ok := docs[0].(map[string]interface{})
	if !ok {
		return ""
	}
	parts := make([]string, 0, 3)
	for _, field := range []string{"progress_msg", "process_msg", "message"} {
		if msg, ok := doc[field].(string); ok && msg != "" {
			parts = append(parts, msg)
		}
	}
	return strings.Join(parts, " | ")
}

func extractDocError(result map[string]interface{}) string {
	data, ok := result["data"]
	if !ok {
		return ""
	}
	d, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}
	docs, ok := d["docs"].([]interface{})
	if !ok || len(docs) == 0 {
		return ""
	}
	doc, ok := docs[0].(map[string]interface{})
	if !ok {
		return ""
	}
	for _, field := range []string{"error", "error_msg", "message", "process_msg", "progress_msg"} {
		if msg, ok := doc[field].(string); ok && strings.TrimSpace(msg) != "" {
			return msg
		}
	}
	return ""
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

	title := "文档解析完成"
	switch snap.ResultStatus {
	case "partial_failed":
		title = "文档解析部分失败"
	case "failed":
		title = "文档解析失败"
	}

	message := fmt.Sprintf("%s\n总计: %d | 成功: %d | 失败: %d\n耗时: %s",
		title, snap.Total, snap.SucceededCount, snap.FinalFailedCount, duration)
	html := fmt.Sprintf("<b>%s</b><br>总计: %d | 成功: %d | 失败: %d<br>耗时: %s",
		title, snap.Total, snap.SucceededCount, snap.FinalFailedCount, duration)

	if snap.RecoveringCount > 0 {
		message += fmt.Sprintf("\n恢复中: %d", snap.RecoveringCount)
		html += fmt.Sprintf("<br>恢复中: %d", snap.RecoveringCount)
	}

	if len(snap.FailedDocs) > 0 {
		message += fmt.Sprintf("\n\n最终失败文档 (%d):", len(snap.FailedDocs))
		html += fmt.Sprintf("<br><br><b>最终失败文档 (%d):</b><ul>", len(snap.FailedDocs))
		limit := len(snap.FailedDocs)
		if limit > 10 {
			limit = 10
		}
		for _, fd := range snap.FailedDocs[:limit] {
			message += fmt.Sprintf("\n- %s: %s (恢复 %d 次)", shortDocID(fd.DocumentID), fd.Error, fd.Retries)
			html += fmt.Sprintf("<li>%s: %s (恢复 %d 次)</li>", shortDocID(fd.DocumentID), fd.Error, fd.Retries)
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

func deriveResultStatus(success, failed, total int) string {
	switch {
	case total == 0:
		return "success"
	case failed == 0:
		return "success"
	case success == 0:
		return "failed"
	default:
		return "partial_failed"
	}
}

func taskDocKey(datasetID, docID string) string {
	return datasetID + ":" + docID
}

func upsertFailedDoc(list []FailedDoc, fd FailedDoc) []FailedDoc {
	for i := range list {
		if list[i].DatasetID == fd.DatasetID && list[i].DocumentID == fd.DocumentID {
			list[i] = fd
			return list
		}
	}
	return append(list, fd)
}

func removeFailedDoc(list []FailedDoc, datasetID, docID string) []FailedDoc {
	out := list[:0]
	for _, fd := range list {
		if fd.DatasetID == datasetID && fd.DocumentID == docID {
			continue
		}
		out = append(out, fd)
	}
	return out
}

func containsDoc(list []string, docID string) bool {
	for _, item := range list {
		if item == docID {
			return true
		}
	}
	return false
}

func removeDoc(list []string, docID string) []string {
	out := list[:0]
	for _, item := range list {
		if item == docID {
			continue
		}
		out = append(out, item)
	}
	return out
}

func shortDocID(docID string) string {
	if len(docID) > 8 {
		return docID[:8]
	}
	return docID
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
