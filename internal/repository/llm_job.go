package repository

import (
	"errors"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LLMJobRepository struct {
	db *gorm.DB
}

func NewLLMJobRepository(db *gorm.DB) *LLMJobRepository {
	return &LLMJobRepository{db: db}
}

func (r *LLMJobRepository) Enqueue(job *model.LLMJob) (*model.LLMJob, error) {
	if job.Status == "" {
		job.Status = model.LLMJobPending
	}
	if job.MaxRetries <= 0 {
		job.MaxRetries = 12
	}
	if job.IdempotencyKey != "" {
		var existing model.LLMJob
		err := r.db.Where("idempotency_key = ? AND status NOT IN ?", job.IdempotencyKey,
			[]model.LLMJobStatus{model.LLMJobSuccess, model.LLMJobFailed, model.LLMJobDead}).
			First(&existing).Error
		if err == nil {
			return &existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := r.db.Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

func (r *LLMJobRepository) Dequeue() (*model.LLMJob, error) {
	var job model.LLMJob
	err := r.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ?", []model.LLMJobStatus{model.LLMJobPending, model.LLMJobRetrying}).
			Where("next_retry_at IS NULL OR next_retry_at <= ?", time.Now()).
			Order("priority DESC, created_at ASC, id ASC").
			First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&job).Updates(map[string]interface{}{
			"status":     model.LLMJobRunning,
			"started_at": &now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	if job.ID == 0 {
		return nil, nil
	}
	job.Status = model.LLMJobRunning
	return &job, nil
}

func (r *LLMJobRepository) Get(id uint) (*model.LLMJob, error) {
	var job model.LLMJob
	if err := r.db.First(&job, id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// DequeueByID 原子 claim 指定 job（1.0 事件化：NATS 消息携带 job_id，消费者据此 claim）。
//
// 仅当 job 处于 pending/retrying 且 next_retry_at 到期时，UPDATE 为 running 并返回；
// 否则返回 nil（job 已被其他消费者 claim 或已终结，调用方应直接 Ack 丢弃事件）。
// 这保证即便同一 job 被发多次事件（EnqueueChat + recoveryLoop 重投），只有一个消费者拿到。
func (r *LLMJobRepository) DequeueByID(id uint) (*model.LLMJob, error) {
	var job model.LLMJob
	err := r.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("id = ?", id).
			Where("status IN ?", []model.LLMJobStatus{model.LLMJobPending, model.LLMJobRetrying}).
			Where("next_retry_at IS NULL OR next_retry_at <= ?", time.Now()).
			First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&job).Updates(map[string]interface{}{
			"status":     model.LLMJobRunning,
			"started_at": &now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	if job.ID == 0 {
		return nil, nil
	}
	job.Status = model.LLMJobRunning
	return &job, nil
}

func (r *LLMJobRepository) MarkSuccess(id uint, response string) error {
	now := time.Now()
	return r.db.Model(&model.LLMJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":        model.LLMJobSuccess,
		"response_text": response,
		"error_class":   "",
		"error_message": "",
		"finished_at":   &now,
	}).Error
}

func (r *LLMJobRepository) MarkRetry(id uint, nextRetryAt time.Time, errorClass, errorMessage string) error {
	return r.db.Model(&model.LLMJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":        model.LLMJobRetrying,
		"retry_count":   gorm.Expr("retry_count + 1"),
		"next_retry_at": nextRetryAt,
		"error_class":   errorClass,
		"error_message": errorMessage,
	}).Error
}

func (r *LLMJobRepository) MarkDead(id uint, errorClass, errorMessage string) error {
	now := time.Now()
	return r.db.Model(&model.LLMJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":        model.LLMJobDead,
		"error_class":   errorClass,
		"error_message": errorMessage,
		"finished_at":   &now,
	}).Error
}

func (r *LLMJobRepository) RecoverRunning(staleAfter time.Duration) (int64, error) {
	cutoff := time.Now().Add(-staleAfter)
	nextRetry := time.Now()
	result := r.db.Model(&model.LLMJob{}).
		Where("status = ? AND started_at < ?", model.LLMJobRunning, cutoff).
		Updates(map[string]interface{}{
			"status":        model.LLMJobRetrying,
			"next_retry_at": &nextRetry,
			"error_class":   "stale_recovered",
			"error_message": "job was running during worker shutdown or crash",
		})
	return result.RowsAffected, result.Error
}

// ListReadyIDs 返回到期待处理的 pending/retrying job ID 列表（1.0 事件化：
// recoveryLoop 据此重发 llm.job.submit 事件，兜底 EnqueueChat 时事件发布失败
// 或 worker 崩溃导致 job 卡住的场景）。limit 限制单批重投数量。
func (r *LLMJobRepository) ListReadyIDs(limit int) ([]uint, error) {
	if limit <= 0 {
		limit = 100
	}
	var ids []uint
	err := r.db.Model(&model.LLMJob{}).
		Where("status IN ?", []model.LLMJobStatus{model.LLMJobPending, model.LLMJobRetrying}).
		Where("next_retry_at IS NULL OR next_retry_at <= ?", time.Now()).
		Order("priority DESC, created_at ASC, id ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}
