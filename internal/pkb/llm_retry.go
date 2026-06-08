package pkb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/service"
)

func (c *Curator) chatCompletionWithRetry(model, systemPrompt, userPrompt string, temperature float64, taskType string) (string, error) {
	if c.llmJobs != nil {
		return c.chatCompletionViaQueue(model, systemPrompt, userPrompt, temperature, taskType)
	}
	retry := c.domains.Defaults.Retry
	maxAttempts := retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	initialBackoff := time.Duration(retry.InitialBackoffSeconds) * time.Second
	if initialBackoff <= 0 {
		initialBackoff = 20 * time.Second
	}
	maxBackoff := time.Duration(retry.MaxBackoffSeconds) * time.Second
	if maxBackoff <= 0 {
		maxBackoff = 300 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		out, err := c.client.ChatCompletion(model, systemPrompt, userPrompt, temperature, taskType)
		if err == nil {
			return out, nil
		}
		lastErr = err
		wait, retryable := llmclient.RetryDelay(err, attempt, initialBackoff, maxBackoff)
		if !retryable || attempt == maxAttempts {
			break
		}
		fmt.Printf("    ↻ LLM 限流/繁忙，等待 %s 后重试（%d/%d）\n", wait, attempt+1, maxAttempts)
		time.Sleep(wait)
	}
	return "", lastErr
}

func (c *Curator) chatCompletionViaQueue(modelName, systemPrompt, userPrompt string, temperature float64, taskType string) (string, error) {
	messages := make([]llmclient.ChatMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, llmclient.ChatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, llmclient.ChatMessage{Role: "user", Content: userPrompt})
	job, err := c.llmJobs.EnqueueChat(service.EnqueueLLMChatOptions{
		TaskType:       taskType,
		CallerID:       "pkb-curate",
		Model:          modelName,
		Messages:       messages,
		Temperature:    temperature,
		Priority:       pkbTaskPriority(taskType),
		IdempotencyKey: pkbLLMIdempotencyKey(modelName, taskType, systemPrompt, userPrompt),
	})
	if err != nil {
		return "", fmt.Errorf("enqueue llm job: %w", err)
	}
	fmt.Printf("    ↪ LLM job queued id=%d type=%s model=%s status=%s\n", job.ID, taskType, modelName, job.Status)
	waitCtx, cancel := context.WithTimeout(c.ctx, pkbQueueWaitTimeout(c.domains.Defaults.Retry))
	defer cancel()
	done, err := c.llmJobs.Wait(waitCtx, job.ID, 2*time.Second)
	if err != nil {
		return "", fmt.Errorf("wait llm job %d: %w", job.ID, err)
	}
	if done.Status != model.LLMJobSuccess {
		return "", service.LLMJobTerminalError(done)
	}
	return done.ResponseText, nil
}

func isRetryableLLMError(err error) bool {
	return llmclient.IsRetryable(err)
}

func isBudgetExhausted(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "budget exhausted")
}

func pkbTaskPriority(taskType string) int {
	switch taskType {
	case "summary":
		return 20
	case "long_context":
		return 10
	default:
		return 0
	}
}

func pkbLLMIdempotencyKey(modelName, taskType, systemPrompt, userPrompt string) string {
	h := sha256.Sum256([]byte(modelName + "\x00" + taskType + "\x00" + systemPrompt + "\x00" + userPrompt))
	return "pkb:" + hex.EncodeToString(h[:])
}

func pkbQueueWaitTimeout(retry Retry) time.Duration {
	maxBackoff := time.Duration(retry.MaxBackoffSeconds) * time.Second
	if maxBackoff <= 0 {
		maxBackoff = 15 * time.Minute
	}
	attempts := retry.MaxAttempts
	if attempts < 1 {
		attempts = 24
	}
	timeout := time.Duration(attempts+1) * maxBackoff
	if timeout < time.Hour {
		return time.Hour
	}
	if timeout > 24*time.Hour {
		return 24 * time.Hour
	}
	return timeout
}
