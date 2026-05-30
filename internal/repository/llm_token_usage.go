package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
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

func (r *LLMTokenUsageRepository) AddUsage(tokenID uint, date time.Time, requests, promptTokens, completionTokens, cachedTokens, costCents, errorCount int) error {
	usage, err := r.GetOrCreate(tokenID, date)
	if err != nil {
		return err
	}
	usage.Requests += requests
	usage.PromptTokens += promptTokens
	usage.CompletionTokens += completionTokens
	usage.CachedTokens += cachedTokens
	usage.CostCents += costCents
	usage.ErrorCount += errorCount
	return r.db.Save(usage).Error
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

// Aggregate returns usage aggregated by the given dimension (token, date, model).
func (r *LLMTokenUsageRepository) Aggregate(groupBy string, from, to time.Time) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	var selectCols string
	switch groupBy {
	case "token":
		selectCols = "token_id, SUM(requests) as requests, SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens, SUM(cached_tokens) as cached_tokens, SUM(cost_cents) as cost_cents, SUM(error_count) as error_count"
	case "date":
		selectCols = "date, SUM(requests) as requests, SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens, SUM(cached_tokens) as cached_tokens, SUM(cost_cents) as cost_cents, SUM(error_count) as error_count"
	case "model":
		// model aggregation requires joining through tokens; for now aggregate by token_id as proxy
		selectCols = "token_id, SUM(requests) as requests, SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens, SUM(cached_tokens) as cached_tokens, SUM(cost_cents) as cost_cents, SUM(error_count) as error_count"
	default:
		selectCols = "token_id, date, SUM(requests) as requests, SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens, SUM(cached_tokens) as cached_tokens, SUM(cost_cents) as cost_cents, SUM(error_count) as error_count"
	}

	rows, err := r.db.Model(&model.LLMTokenUsageDaily{}).
		Select(selectCols).
		Where("date >= ? AND date <= ?", from, to).
		Group(groupBy).
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
