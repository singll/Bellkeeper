package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LLMTokenUsageRepository struct {
	db *gorm.DB
}

func NewLLMTokenUsageRepository(db *gorm.DB) *LLMTokenUsageRepository {
	return &LLMTokenUsageRepository{db: db}
}

func (r *LLMTokenUsageRepository) GetOrCreate(tokenID uint, date time.Time) (*model.LLMTokenUsageDaily, error) {
	var usage model.LLMTokenUsageDaily
	err := r.db.Where("token_id = ? AND date = ?", tokenID, date).First(&usage).Error
	if err == nil {
		return &usage, nil
	}
	if err == gorm.ErrRecordNotFound {
		usage = model.LLMTokenUsageDaily{
			TokenID: tokenID,
			Date:    date,
		}
		if err := r.db.Create(&usage).Error; err != nil {
			return nil, err
		}
		return &usage, nil
	}
	return nil, err
}

// AddUsage atomically upserts a daily usage row, incrementing counters in a single
// statement (Postgres INSERT ... ON CONFLICT DO UPDATE). This avoids the read-
// modify-write race where concurrent requests for the same (token_id, date) lose
// increments. Requires the composite unique index idx_llm_usage_token_date.
func (r *LLMTokenUsageRepository) AddUsage(tokenID uint, date time.Time, requests, promptTokens, completionTokens, cachedTokens, costCents int, costMicroCents int64, errorCount int) error {
	row := model.LLMTokenUsageDaily{
		TokenID:          tokenID,
		Date:             date,
		Requests:         requests,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		CachedTokens:     cachedTokens,
		CostCents:        costCents,
		CostMicroCents:   costMicroCents,
		ErrorCount:       errorCount,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "token_id"}, {Name: "date"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"requests":          gorm.Expr("llm_token_usage_daily.requests + EXCLUDED.requests"),
			"prompt_tokens":     gorm.Expr("llm_token_usage_daily.prompt_tokens + EXCLUDED.prompt_tokens"),
			"completion_tokens": gorm.Expr("llm_token_usage_daily.completion_tokens + EXCLUDED.completion_tokens"),
			"cached_tokens":     gorm.Expr("llm_token_usage_daily.cached_tokens + EXCLUDED.cached_tokens"),
			"cost_cents":        gorm.Expr("llm_token_usage_daily.cost_cents + EXCLUDED.cost_cents"),
			"cost_micro_cents":  gorm.Expr("llm_token_usage_daily.cost_micro_cents + EXCLUDED.cost_micro_cents"),
			"error_count":       gorm.Expr("llm_token_usage_daily.error_count + EXCLUDED.error_count"),
		}),
	}).Create(&row).Error
}

func (r *LLMTokenUsageRepository) ListByToken(tokenID uint, from, to time.Time) ([]model.LLMTokenUsageDaily, error) {
	var usages []model.LLMTokenUsageDaily
	if err := r.db.Where("token_id = ? AND date >= ? AND date <= ?", tokenID, from, to).
		Order("date ASC").Find(&usages).Error; err != nil {
		return nil, err
	}
	return usages, nil
}

func (r *LLMTokenUsageRepository) ListByDateRange(from, to time.Time) ([]model.LLMTokenUsageDaily, error) {
	var usages []model.LLMTokenUsageDaily
	if err := r.db.Where("date >= ? AND date <= ?", from, to).
		Order("date ASC, token_id ASC").Find(&usages).Error; err != nil {
		return nil, err
	}
	return usages, nil
}

// Aggregate returns usage aggregated by the given dimension (token or date).
// Note: group_by=model is NOT served here (llm_token_usage_daily has no model
// column) — the service routes that to LLMProxyRepository.AggregateByModel.
func (r *LLMTokenUsageRepository) Aggregate(groupBy string, from, to time.Time) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	sumCols := "SUM(requests) as requests, SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens, SUM(cached_tokens) as cached_tokens, SUM(cost_cents) as cost_cents, SUM(cost_micro_cents) as cost_micro_cents, SUM(error_count) as error_count"
	var selectCols, groupCol string
	switch groupBy {
	case "token":
		selectCols = "token_id, " + sumCols
		groupCol = "token_id"
	case "date":
		selectCols = "date, " + sumCols
		groupCol = "date"
	default:
		selectCols = "token_id, date, " + sumCols
		groupCol = "token_id, date"
	}

	rows, err := r.db.Model(&model.LLMTokenUsageDaily{}).
		Select(selectCols).
		Where("date >= ? AND date <= ?", from, to).
		Group(groupCol).
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}
		row := make(map[string]interface{})
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}
	return results, rows.Err()
}
