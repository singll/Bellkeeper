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
