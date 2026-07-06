package llmgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/eventbus"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

type LLMJobQueueService struct {
	cfg  config.LLMJobQueueConfig
	repo *repository.LLMJobRepository
	llm  Gateway
	bus  *eventbus.Client // 1.0 事件化：nil 时降级为 DB 轮询（行为不变）

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// LLMJobSubmitPayload 是 llm.job.submit 事件的 payload（仅携带 job_id，DB 为状态真相源）。
type LLMJobSubmitPayload struct {
	JobID uint `json:"job_id"`
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
	// 1.0 §2.1.3：结构化输出 + 长度限制（透传给 LLMChatJobPayload.Request）。
	ResponseFormat *llmclient.ResponseFormat
	MaxTokens      int
}

// NewLLMJobQueueService 构造 LLM 任务队列。bus 非 nil 时启用事件驱动（NATS llm.job.submit），
// nil 时降级为 DB 轮询（行为等同 1.0 重构前）。bus 亦可在 Start 前经 SetEventBus 注入。
func NewLLMJobQueueService(cfg config.LLMJobQueueConfig, repo *repository.LLMJobRepository, llm Gateway, bus *eventbus.Client) *LLMJobQueueService {
	return &LLMJobQueueService{
		cfg:  cfg,
		repo: repo,
		llm:  llm,
		bus:  bus,
	}
}

// SetEventBus 在 Start 前注入事件总线（供 app.go 在 eventBus 创建后接线）。
func (s *LLMJobQueueService) SetEventBus(bus *eventbus.Client) {
	s.bus = bus
}

func (s *LLMJobQueueService) EnqueueChat(opts EnqueueLLMChatOptions) (*model.LLMJob, error) {
	payload := LLMChatJobPayload{
		Request: llmclient.ChatRequest{
			Model:          opts.Model,
			Messages:       opts.Messages,
			Temperature:    opts.Temperature,
			ResponseFormat: opts.ResponseFormat,
			MaxTokens:      opts.MaxTokens,
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
	job, err := s.repo.Enqueue(&model.LLMJob{
		TaskType:       opts.TaskType,
		CallerID:       opts.CallerID,
		Model:          opts.Model,
		Status:         model.LLMJobPending,
		Priority:       opts.Priority,
		IdempotencyKey: opts.IdempotencyKey,
		RequestJSON:    raw,
		MaxRetries:     maxRetries,
	})
	if err != nil {
		return nil, err
	}
	// 1.0 事件化：DB 入队后发 llm.job.submit，驱动 worker 消费（替代 DB 轮询）。
	// 事件发布失败不阻断 EnqueueChat（recoveryLoop 会兜底重投到期 pending job）。
	if s.bus != nil {
		ev, perr := eventbus.New(context.Background(), "llm.job.submit", eventbus.SourceLLM, fmt.Sprintf("llmjob:%d", job.ID), LLMJobSubmitPayload{JobID: job.ID})
		if perr == nil {
			if perr := s.bus.PublishEvent(context.Background(), ev); perr != nil {
				log.Printf("[LLMJobQueue] publish llm.job.submit failed for job %d: %v (recovery loop will retry)", job.ID, perr)
			}
		}
	}
	return job, nil
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
	// Heartbeat goroutine
	s.wg.Add(1)
	go s.heartbeatLoop(ctx)
	workers := s.cfg.Workers
	if workers <= 0 {
		workers = 1
	}
	if s.bus != nil {
		// 1.0 事件驱动：NATS llm.job.submit 消费者。
		sub, err := s.bus.Subscribe("llm.job.submit", "bellkeeper-llm-worker")
		if err != nil {
			log.Printf("[LLMJobQueue] subscribe llm.job.submit failed: %v — fallback to DB polling", err)
			s.startPollingWorkers(ctx, workers)
			return
		}
		for i := 0; i < workers; i++ {
			s.wg.Add(1)
			go s.eventWorkerLoop(ctx, i+1, sub)
		}
		log.Printf("[LLMJobQueue] started (event-driven): workers=%d", workers)
		return
	}
	// Fallback：DB 轮询（eventbus 未配置）。
	s.startPollingWorkers(ctx, workers)
	log.Printf("[LLMJobQueue] started (db-polling): workers=%d poll=%s", workers, s.pollInterval())
}

func (s *LLMJobQueueService) startPollingWorkers(ctx context.Context, workers int) {
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.workerLoop(ctx, i+1)
	}
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

// eventWorkerLoop 是 1.0 事件驱动的 worker：PullSubscribe + Fetch llm.job.submit，
// 收到 job_id 后 DequeueByID 原子 claim，再调 processJob。
//
// 同一 job 可能被发多次事件（EnqueueChat + recoveryLoop 重投），DB DequeueByID
// 的原子 UPDATE 保证只有一个消费者 claim 成功，其他收到 nil 直接 Ack 丢弃。
func (s *LLMJobQueueService) eventWorkerLoop(ctx context.Context, id int, sub *nats.Subscription) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msgs, err := sub.Fetch(10, nats.MaxWait(5*time.Second))
		if err != nil {
			if err == nats.ErrTimeout {
				continue
			}
			middleware.GetLogger().Warn("LLMJobQueue: fetch error", zap.Int("worker", id), zap.Error(err))
			time.Sleep(time.Second)
			continue
		}
		for _, msg := range msgs {
			s.processEventMessage(ctx, msg)
		}
	}
}

func (s *LLMJobQueueService) processEventMessage(ctx context.Context, msg *nats.Msg) {
	ev, err := eventbus.UnmarshalEvent(msg.Data)
	if err != nil {
		middleware.GetLogger().Error("LLMJobQueue: unmarshal event envelope", zap.Error(err))
		_ = msg.Nak()
		return
	}
	var payload LLMJobSubmitPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		middleware.GetLogger().Error("LLMJobQueue: unmarshal submit payload", zap.Error(err))
		_ = msg.Nak()
		return
	}
	// 原子 claim：仅当 pending/retrying 且到期时 UPDATE 为 running。
	job, err := s.repo.DequeueByID(payload.JobID)
	if err != nil {
		middleware.GetLogger().Error("LLMJobQueue: dequeue by id", zap.Uint("job_id", payload.JobID), zap.Error(err))
		_ = msg.NakWithDelay(10 * time.Second)
		return
	}
	if job == nil {
		// 已被其他消费者 claim 或已终结 → Ack 丢弃事件（不重复处理）。
		_ = msg.Ack()
		return
	}
	s.processJob(ctx, job)
	_ = msg.Ack()
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
	out, err := s.llm.Chat(ctx, payload.Request, payload.Options)
	if err == nil {
		_ = s.repo.MarkSuccess(job.ID, out.Content)
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
			// 1.0 事件化：重发到期 pending/retrying job 的 llm.job.submit 事件，
			// 兜底 EnqueueChat 时事件丢失或 worker 崩溃后 job 卡住。DB 轮询模式下
			// 这些 job 由 workerLoop 主动 Dequeue，无需重发（重发也无消费者）。
			if s.bus != nil {
				s.republishReadyJobs(ctx)
			}
		}
	}
}

// republishReadyJobs 查询到期 pending/retrying job 并重发 llm.job.submit。
// 幂等：DB DequeueByID 保证重复事件只有一个消费者 claim 成功。
func (s *LLMJobQueueService) republishReadyJobs(ctx context.Context) {
	ids, err := s.repo.ListReadyIDs(100)
	if err != nil {
		log.Printf("[LLMJobQueue] list ready jobs error: %v", err)
		return
	}
	for _, id := range ids {
		ev, perr := eventbus.New(ctx, "llm.job.submit", eventbus.SourceLLM, fmt.Sprintf("llmjob:%d", id), LLMJobSubmitPayload{JobID: id})
		if perr != nil {
			continue
		}
		if perr := s.bus.PublishEvent(ctx, ev); perr != nil {
			log.Printf("[LLMJobQueue] republish llm.job.submit failed for job %d: %v", id, perr)
		}
	}
}

func (s *LLMJobQueueService) heartbeatLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Printf("[LLMJobQueue] heartbeat: alive")
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
