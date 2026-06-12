package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLLMJobRepository_EnqueueAndGet(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMJobRepository(db)

	job := &model.LLMJob{
		TaskType:  "summarize",
		CallerID:  "caller-1",
		Model:     "gpt-4o",
		Priority:  5,
		Status:    model.LLMJobPending,
		MaxRetries: 12,
	}
	result, err := repo.Enqueue(job)
	assertNoError(t, err, "Enqueue")
	assertEqual(t, result.ID > 0, true)
	assertEqual(t, result.Status, model.LLMJobPending)

	got, err := repo.Get(job.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.TaskType, "summarize")
	assertEqual(t, got.CallerID, "caller-1")
}

func TestLLMJobRepository_EnqueueIdempotent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMJobRepository(db)

	job1 := &model.LLMJob{
		TaskType:       "summarize",
		CallerID:       "caller-1",
		Model:          "gpt-4o",
		IdempotencyKey: "idem-1",
		Status:         model.LLMJobPending,
	}
	result1, err := repo.Enqueue(job1)
	assertNoError(t, err, "Enqueue 1")

	job2 := &model.LLMJob{
		TaskType:       "summarize",
		CallerID:       "caller-1",
		Model:          "gpt-4o",
		IdempotencyKey: "idem-1",
		Status:         model.LLMJobPending,
	}
	result2, err := repo.Enqueue(job2)
	assertNoError(t, err, "Enqueue 2 idempotent")
	assertEqual(t, result2.ID, result1.ID)
}

func TestLLMJobRepository_EnqueueDefaults(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMJobRepository(db)

	job := &model.LLMJob{
		TaskType: "test",
		CallerID: "c1",
		Model:    "m1",
	}
	result, err := repo.Enqueue(job)
	assertNoError(t, err, "Enqueue")
	assertEqual(t, result.Status, model.LLMJobPending)
	assertEqual(t, result.MaxRetries, 12)
}

func TestLLMJobRepository_MarkSuccess(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMJobRepository(db)

	job := &model.LLMJob{TaskType: "t", CallerID: "c", Model: "m", Status: model.LLMJobRunning}
	_, err := repo.Enqueue(job)
	assertNoError(t, err, "Enqueue")

	assertNoError(t, repo.MarkSuccess(job.ID, "result text"), "MarkSuccess")

	got, err := repo.Get(job.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.Status, model.LLMJobSuccess)
	assertEqual(t, got.ResponseText, "result text")
	assertEqual(t, got.FinishedAt != nil, true)
}

func TestLLMJobRepository_MarkDead(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMJobRepository(db)

	job := &model.LLMJob{TaskType: "t", CallerID: "c", Model: "m", Status: model.LLMJobRunning}
	_, err := repo.Enqueue(job)
	assertNoError(t, err, "Enqueue")

	assertNoError(t, repo.MarkDead(job.ID, "timeout", "job timed out"), "MarkDead")

	got, err := repo.Get(job.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.Status, model.LLMJobDead)
	assertEqual(t, got.ErrorClass, "timeout")
	assertEqual(t, got.FinishedAt != nil, true)
}

func TestLLMJobRepository_RecoverRunning(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMJobRepository(db)

	job := &model.LLMJob{TaskType: "t", CallerID: "c", Model: "m", Status: model.LLMJobRunning}
	_, err := repo.Enqueue(job)
	assertNoError(t, err, "Enqueue")

	past := time.Now().Add(-5 * time.Minute)
	db.Model(&model.LLMJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"status": model.LLMJobRunning, "started_at": past,
	})

	affected, err := repo.RecoverRunning(1 * time.Minute)
	assertNoError(t, err, "RecoverRunning")
	assertEqual(t, affected, int64(1))

	got, err := repo.Get(job.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.Status, model.LLMJobRetrying)
}

func TestLLMJobRepository_Dequeue(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMJobRepository(db)

	job := &model.LLMJob{TaskType: "t", CallerID: "c", Model: "m", Status: model.LLMJobPending, Priority: 5}
	_, err := repo.Enqueue(job)
	assertNoError(t, err, "Enqueue")

	dequeued, err := repo.Dequeue()
	assertNoError(t, err, "Dequeue")
	assertEqual(t, dequeued != nil, true)
	assertEqual(t, dequeued.ID, job.ID)
	assertEqual(t, dequeued.Status, model.LLMJobRunning)

	dequeued2, err := repo.Dequeue()
	assertNoError(t, err, "Dequeue empty")
	assertEqual(t, dequeued2, nil)
}

func TestLLMJobRepository_MarkRetry(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMJobRepository(db)

	job := &model.LLMJob{TaskType: "t", CallerID: "c", Model: "m", Status: model.LLMJobRunning}
	_, err := repo.Enqueue(job)
	assertNoError(t, err, "Enqueue")

	nextRetry := time.Now().Add(30 * time.Second)
	assertNoError(t, repo.MarkRetry(job.ID, nextRetry, "timeout", "job timed out"), "MarkRetry")

	got, err := repo.Get(job.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.Status, model.LLMJobRetrying)
	assertEqual(t, got.RetryCount, 1)
	assertEqual(t, got.ErrorClass, "timeout")
}
