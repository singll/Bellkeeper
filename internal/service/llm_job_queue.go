package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type LLMJobQueueService struct {
	cfg    config.LLMJobQueueConfig
	repo   *repository.LLMJobRepository
	client *llmclient.Client

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type LLMChatJobPayload struct {
	Request llmclient.ChatRequest `json:"request"`
	Options llmclient.ChatOptions `json:"options"`
}

type EnqueueLLMChatOptions struct {
	TaskType       string
	CallerID       string
	Model          string
	Messages       []llmclient.ChatMessage
	Temperature    float64
	Priority       int
	IdempotencyKey string
}

func NewLLMJobQueueService(cfg config.LLMJobQueueConfig, repo *repository.LLMJobRepository, llmBaseURL, apiKey string) *LLMJobQueueService {
	timeout := time.Duration(cfg.JobTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &LLMJobQueueService{
		cfg:  cfg,
		repo: repo,
		client: llmclient.New(llmclient.Options{
			BaseURL: llmBaseURL,
			APIKey:  apiKey,
			Timeout: timeout,
		}),
	}
}

func (s *LLMJobQueueService) EnqueueChat(opts EnqueueLLMChatOptions) (*model.LLMJob, error) {
	payload := LLMChatJobPayload{
		Request: llmclient.ChatRequest{
			Model:       opts.Model,
			Messages:    opts.Messages,
			Temperature: opts.Temperature,
		},
		Options: llmclient.ChatOptions{
			CallerID: opts.CallerID,
			TaskType: opts.TaskType,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	maxRetries := optsMaxRetries(s.cfg.MaxRetries)
	return s.repo.Enqueue(&model.LLMJob{
		TaskType:       opts.TaskType,
		CallerID:       opts.CallerID,
		Model:          opts.Model,
		Status:         model.LLMJobPending,
		Priority:       opts.Priority,
		IdempotencyKey: opts.IdempotencyKey,
		RequestJSON:    raw,
		MaxRetries:     maxRetries,
	})
}

func (s *LLMJobQueueService) Wait(ctx context.Context, jobID uint, pollInterval time.Duration) (*model.LLMJob, error) {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		job, err := s.repo.Get(jobID)
		if err != nil {
			return nil, err
		}
		if job.Status.IsTerminal() {
			return job, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *LLMJobQueueService) Start(ctx context.Context) {
	if !s.cfg.Enabled || s.repo == nil {
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	if recovered, err := s.repo.RecoverRunning(s.staleTimeout()); err != nil {
		log.Printf("[LLMJobQueue] recover running jobs failed: %v", err)
	} else if recovered > 0 {
		log.Printf("[LLMJobQueue] recovered %d stale running jobs", recovered)
	}
	s.wg.Add(1)
	go s.recoveryLoop(ctx)
	workers := s.cfg.Workers
	if workers <= 0 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.workerLoop(ctx, i+1)
	}
	log.Printf("[LLMJobQueue] started: workers=%d poll=%s", workers, s.pollInterval())
}

func (s *LLMJobQueueService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	log.Printf("[LLMJobQueue] all workers stopped")
}

func (s *LLMJobQueueService) workerLoop(ctx context.Context, id int) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.pollInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		job, err := s.repo.Dequeue()
		if err != nil {
			log.Printf("[LLMJobQueue:%d] dequeue error: %v", id, err)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		if job == nil {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		s.processJob(ctx, job)
	}
}

func (s *LLMJobQueueService) processJob(parent context.Context, job *model.LLMJob) {
	var payload LLMChatJobPayload
	if err := json.Unmarshal(job.RequestJSON, &payload); err != nil {
		_ = s.repo.MarkDead(job.ID, "bad_payload", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(s.cfg.JobTimeoutSeconds)*time.Second)
	if s.cfg.JobTimeoutSeconds <= 0 {
		ctx, cancel = context.WithTimeout(parent, 10*time.Minute)
	}
	defer cancel()
	out, err := s.client.ChatCompletion(ctx, payload.Request, payload.Options)
	if err == nil {
		_ = s.repo.MarkSuccess(job.ID, out)
		return
	}
	errClass := llmclient.ErrorClass(err)
	if job.RetryCount >= job.MaxRetries || !llmclient.IsRetryable(err) {
		_ = s.repo.MarkDead(job.ID, errClass, err.Error())
		return
	}
	wait, _ := llmclient.RetryDelay(err, job.RetryCount+1, s.initialBackoff(), s.maxBackoff())
	next := time.Now().Add(wait)
	_ = s.repo.MarkRetry(job.ID, next, errClass, err.Error())
	log.Printf("[LLMJobQueue] retry job=%d type=%s attempt=%d next=%s err=%v",
		job.ID, job.TaskType, job.RetryCount+1, next.Format(time.RFC3339), err)
}

func (s *LLMJobQueueService) recoveryLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.recoveryInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recovered, err := s.repo.RecoverRunning(s.staleTimeout())
			if err != nil {
				log.Printf("[LLMJobQueue] stale recovery error: %v", err)
			} else if recovered > 0 {
				log.Printf("[LLMJobQueue] recovered %d stale running jobs", recovered)
			}
		}
	}
}

func (s *LLMJobQueueService) pollInterval() time.Duration {
	if s.cfg.PollIntervalSeconds <= 0 {
		return 5 * time.Second
	}
	return time.Duration(s.cfg.PollIntervalSeconds) * time.Second
}

func (s *LLMJobQueueService) initialBackoff() time.Duration {
	if s.cfg.InitialBackoffSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(s.cfg.InitialBackoffSeconds) * time.Second
}

func (s *LLMJobQueueService) maxBackoff() time.Duration {
	if s.cfg.MaxBackoffSeconds <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(s.cfg.MaxBackoffSeconds) * time.Second
}

func (s *LLMJobQueueService) staleTimeout() time.Duration {
	if s.cfg.StaleTimeoutMinutes <= 0 {
		return 30 * time.Minute
	}
	return time.Duration(s.cfg.StaleTimeoutMinutes) * time.Minute
}

func (s *LLMJobQueueService) recoveryInterval() time.Duration {
	if s.cfg.RecoveryIntervalMinutes <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(s.cfg.RecoveryIntervalMinutes) * time.Minute
}

func optsMaxRetries(v int) int {
	if v <= 0 {
		return 12
	}
	return v
}

func LLMJobTerminalError(job *model.LLMJob) error {
	if job == nil {
		return fmt.Errorf("llm job missing")
	}
	if job.Status == model.LLMJobSuccess {
		return nil
	}
	return fmt.Errorf("llm job %d ended with %s (%s): %s", job.ID, job.Status, job.ErrorClass, job.ErrorMessage)
}
