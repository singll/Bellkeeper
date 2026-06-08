package model

import (
	"time"

	"gorm.io/gorm"
)

type LLMJobStatus string

const (
	LLMJobPending  LLMJobStatus = "pending"
	LLMJobRunning  LLMJobStatus = "running"
	LLMJobRetrying LLMJobStatus = "retrying"
	LLMJobSuccess  LLMJobStatus = "success"
	LLMJobFailed   LLMJobStatus = "failed"
	LLMJobDead     LLMJobStatus = "dead"
)

func (s LLMJobStatus) IsTerminal() bool {
	return s == LLMJobSuccess || s == LLMJobFailed || s == LLMJobDead
}

type LLMJob struct {
	ID             uint           `gorm:"primarykey" json:"id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	TaskType       string         `gorm:"size:64;index" json:"task_type"`
	CallerID       string         `gorm:"size:128;index" json:"caller_id"`
	Model          string         `gorm:"size:128;index" json:"model"`
	Status         LLMJobStatus   `gorm:"size:32;default:'pending';index" json:"status"`
	Priority       int            `gorm:"default:0;index" json:"priority"`
	IdempotencyKey string         `gorm:"size:255;index" json:"idempotency_key"`
	RequestJSON    []byte         `gorm:"type:jsonb" json:"request_json,omitempty"`
	ResponseText   string         `gorm:"type:text" json:"response_text,omitempty"`
	ErrorClass     string         `gorm:"size:64;index" json:"error_class,omitempty"`
	ErrorMessage   string         `gorm:"type:text" json:"error_message,omitempty"`
	RetryCount     int            `gorm:"default:0" json:"retry_count"`
	MaxRetries     int            `gorm:"default:12" json:"max_retries"`
	NextRetryAt    *time.Time     `gorm:"index" json:"next_retry_at,omitempty"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
}

func (LLMJob) TableName() string {
	return "llm_jobs"
}
