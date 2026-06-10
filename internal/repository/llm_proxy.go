package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type LLMProxyRepository struct {
	db *gorm.DB
}

func NewLLMProxyRepository(db *gorm.DB) *LLMProxyRepository {
	return &LLMProxyRepository{db: db}
}

func (r *LLMProxyRepository) CreateLog(log *model.LLMProxyLog) error {
	return r.db.Create(log).Error
}

func (r *LLMProxyRepository) GetRecentLogs(channelName string, limit int) ([]model.LLMProxyLog, error) {
	var logs []model.LLMProxyLog
	q := r.db.Model(&model.LLMProxyLog{}).Order("created_at DESC").Limit(limit)
	if channelName != "" {
		q = q.Where("channel_name = ?", channelName)
	}
	if err := q.Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *LLMProxyRepository) GetRateLimitEvents(since time.Time, channelName string) ([]model.LLMProxyLog, error) {
	var logs []model.LLMProxyLog
	q := r.db.Where("is_rate_limit = ? AND created_at > ?", true, since)
	if channelName != "" {
		q = q.Where("channel_name = ?", channelName)
	}
	if err := q.Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

// ChannelStat holds aggregated stats per channel
type ChannelStat struct {
	ChannelName    string  `json:"channel_name"`
	TotalRequests  int64   `json:"total_requests"`
	RateLimitCount int64   `json:"rate_limit_count"`
	AvgDurationMs  float64 `json:"avg_duration_ms"`
	TotalRetries   int64   `json:"total_retries"`
}

// UsageSummary holds aggregated proxy usage over a time window.
type UsageSummary struct {
	TotalRequests int64   `json:"total_requests"`
	ErrorCount    int64   `json:"error_count"`
	RateLimits    int64   `json:"rate_limits"`
	PromptTokens  int64   `json:"prompt_tokens"`
	CompTokens    int64   `json:"comp_tokens"`
	CostCents     int64   `json:"cost_cents"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// SummarySince aggregates request counts, errors, tokens and cost across all
// channels since the given time.
func (r *LLMProxyRepository) SummarySince(since time.Time) (*UsageSummary, error) {
	var summary UsageSummary
	err := r.db.Model(&model.LLMProxyLog{}).
		Select("COUNT(*) as total_requests, "+
			"COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0) as error_count, "+
			"COALESCE(SUM(CASE WHEN is_rate_limit THEN 1 ELSE 0 END), 0) as rate_limits, "+
			"COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, "+
			"COALESCE(SUM(comp_tokens), 0) as comp_tokens, "+
			"CAST(COALESCE(SUM(cost_micro_cents), 0) / 1000 AS BIGINT) as cost_cents, "+
			"COALESCE(AVG(duration_ms), 0) as avg_duration_ms").
		Where("created_at > ?", since).
		Scan(&summary).Error
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (r *LLMProxyRepository) GetStats(since time.Time) ([]ChannelStat, error) {
	var stats []ChannelStat
	err := r.db.Model(&model.LLMProxyLog{}).
		Select("channel_name, COUNT(*) as total_requests, "+
			"SUM(CASE WHEN is_rate_limit THEN 1 ELSE 0 END) as rate_limit_count, "+
			"AVG(duration_ms) as avg_duration_ms, "+
			"SUM(retry_count) as total_retries").
		Where("created_at > ?", since).
		Group("channel_name").
		Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *LLMProxyRepository) CleanOldLogs(olderThanDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := r.db.Where("created_at < ?", cutoff).Delete(&model.LLMProxyLog{})
	return result.RowsAffected, result.Error
}

func (r *LLMProxyRepository) GetLogsBefore(cutoff time.Time) ([]model.LLMProxyLog, error) {
	var logs []model.LLMProxyLog
	if err := r.db.Where("created_at < ?", cutoff).Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *LLMProxyRepository) DeleteLogsBefore(cutoff time.Time) error {
	return r.db.Where("created_at < ?", cutoff).Delete(&model.LLMProxyLog{}).Error
}

// SaveAlertEvent persists an alert event to the database.
func (r *LLMProxyRepository) SaveAlertEvent(event *model.LLMAlertEvent) error {
	return r.db.Create(event).Error
}

// ListAlertEvents returns recent alert events, optionally filtered by severity and
// alert type, newest first.
func (r *LLMProxyRepository) ListAlertEvents(since time.Time, severity, alertType string, limit int) ([]model.LLMAlertEvent, error) {
	var events []model.LLMAlertEvent
	q := r.db.Where("created_at >= ?", since)
	if severity != "" {
		q = q.Where("severity = ?", severity)
	}
	if alertType != "" {
		q = q.Where("alert_type = ?", alertType)
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if err := q.Order("created_at DESC").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// AggregateByModel returns usage aggregated by model name from the proxy logs.
// llm_token_usage_daily has no model dimension, so model-level billing must come
// from llm_proxy_logs (which records Model + CostMicroCents per request).
func (r *LLMProxyRepository) AggregateByModel(from, to time.Time) ([]map[string]interface{}, error) {
	type modelStat struct {
		Model            string `json:"model"`
		Requests         int64  `json:"requests"`
		PromptTokens     int64  `json:"prompt_tokens"`
		CompletionTokens int64  `json:"completion_tokens"`
		CachedTokens     int64  `json:"cached_tokens"`
		CostCents        int64  `json:"cost_cents"`
		CostMicroCents   int64  `json:"cost_micro_cents"`
		ErrorCount       int64  `json:"error_count"`
	}
	var stats []modelStat
	err := r.db.Model(&model.LLMProxyLog{}).
		Select("model, COUNT(*) as requests, "+
			"COALESCE(SUM(prompt_tokens),0) as prompt_tokens, "+
			"COALESCE(SUM(comp_tokens),0) as completion_tokens, "+
			"COALESCE(SUM(cached_tokens),0) as cached_tokens, "+
			"COALESCE(SUM(cost_cents),0) as cost_cents, "+
			"COALESCE(SUM(cost_micro_cents),0) as cost_micro_cents, "+
			"SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as error_count").
		Where("created_at >= ? AND created_at <= ?", from, to).
		Group("model").
		Order("cost_micro_cents DESC").
		Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	results := make([]map[string]interface{}, 0, len(stats))
	for _, s := range stats {
		results = append(results, map[string]interface{}{
			"model":             s.Model,
			"requests":          s.Requests,
			"prompt_tokens":     s.PromptTokens,
			"completion_tokens": s.CompletionTokens,
			"cached_tokens":     s.CachedTokens,
			"cost_cents":        s.CostCents,
			"cost_micro_cents":  s.CostMicroCents,
			"error_count":       s.ErrorCount,
		})
	}
	return results, nil
}
