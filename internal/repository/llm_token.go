package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type LLMTokenRepository struct {
	db *gorm.DB
}

func NewLLMTokenRepository(db *gorm.DB) *LLMTokenRepository {
	return &LLMTokenRepository{db: db}
}

func (r *LLMTokenRepository) List() ([]model.LLMToken, error) {
	var tokens []model.LLMToken
	if err := r.db.Order("created_at DESC").Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

func (r *LLMTokenRepository) Get(id uint) (*model.LLMToken, error) {
	var t model.LLMToken
	if err := r.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *LLMTokenRepository) GetByKeyHash(hash string) (*model.LLMToken, error) {
	var t model.LLMToken
	if err := r.db.Where("key_hash = ?", hash).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *LLMTokenRepository) GetByCallerID(callerID string) (*model.LLMToken, error) {
	var t model.LLMToken
	if err := r.db.Where("caller_id = ?", callerID).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *LLMTokenRepository) Create(t *model.LLMToken) error {
	return r.db.Create(t).Error
}

func (r *LLMTokenRepository) Update(t *model.LLMToken) error {
	return r.db.Save(t).Error
}

func (r *LLMTokenRepository) Delete(id uint) error {
	return r.db.Delete(&model.LLMToken{}, id).Error
}

func (r *LLMTokenRepository) Count() (int64, error) {
	var count int64
	if err := r.db.Model(&model.LLMToken{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *LLMTokenRepository) CountRequestsToday(tokenID uint) (int, error) {
	var count int64
	today := time.Now().Truncate(24 * time.Hour)
	if err := r.db.Model(&model.LLMProxyLog{}).
		Where("caller_id = (SELECT caller_id FROM llm_tokens WHERE id = ?)", tokenID).
		Where("created_at >= ?", today).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (r *LLMTokenRepository) UpdateLastUsed(tokenID uint) error {
	now := time.Now()
	return r.db.Model(&model.LLMToken{}).Where("id = ?", tokenID).Update("last_used_at", now).Error
}

// TokensUsedToday returns the prompt+completion tokens consumed by a token today,
// from the aggregated daily usage table. Used to enforce QuotaTokensDaily.
func (r *LLMTokenRepository) TokensUsedToday(tokenID uint) (int, error) {
	today := time.Now().Truncate(24 * time.Hour)
	var result struct{ Total int64 }
	err := r.db.Model(&model.LLMTokenUsageDaily{}).
		Select("COALESCE(SUM(prompt_tokens + completion_tokens), 0) as total").
		Where("token_id = ? AND date = ?", tokenID, today).
		Scan(&result).Error
	return int(result.Total), err
}

// CostThisMonthCents returns the month-to-date cost (rounded cents) for a token,
// summed from precise micro-cents. Used to enforce QuotaCostMonthlyCents.
func (r *LLMTokenRepository) CostThisMonthCents(tokenID uint) (int, error) {
	nowUTC := time.Now().UTC()
	monthStart := time.Date(nowUTC.Year(), nowUTC.Month(), 1, 0, 0, 0, 0, time.UTC)
	var result struct{ Total int64 }
	err := r.db.Model(&model.LLMTokenUsageDaily{}).
		Select("COALESCE(SUM(cost_micro_cents), 0) as total").
		Where("token_id = ? AND date >= ?", tokenID, monthStart).
		Scan(&result).Error
	return int((result.Total + 500) / 1000), err // micro-cents → cents, round half up
}
