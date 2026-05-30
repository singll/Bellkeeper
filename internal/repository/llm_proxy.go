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
