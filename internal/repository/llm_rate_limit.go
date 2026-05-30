package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type LLMRateLimitRepository struct {
	db *gorm.DB
}

func NewLLMRateLimitRepository(db *gorm.DB) *LLMRateLimitRepository {
	return &LLMRateLimitRepository{db: db}
}

func (r *LLMRateLimitRepository) GetOrCreate(channelID uint, modelName string, configuredRPM int) (*model.LLMModelRateLimit, error) {
	var rl model.LLMModelRateLimit
	err := r.db.Where("channel_id = ? AND model = ?", channelID, modelName).First(&rl).Error
	if err == nil {
		return &rl, nil
	}
	if err == gorm.ErrRecordNotFound {
		rl = model.LLMModelRateLimit{
			ChannelID:     channelID,
			Model:         modelName,
			ConfiguredRPM: configuredRPM,
			LearnedRPMSafe: int(float64(configuredRPM) * 0.5),
		}
		if err := r.db.Create(&rl).Error; err != nil {
			return nil, err
		}
		return &rl, nil
	}
	return nil, err
}

func (r *LLMRateLimitRepository) Update(rl *model.LLMModelRateLimit) error {
	return r.db.Save(rl).Error
}

func (r *LLMRateLimitRepository) ListByChannel(channelID uint) ([]model.LLMModelRateLimit, error) {
	var rls []model.LLMModelRateLimit
	if err := r.db.Where("channel_id = ?", channelID).Find(&rls).Error; err != nil {
		return nil, err
	}
	return rls, nil
}

func (r *LLMRateLimitRepository) ListAll() ([]model.LLMModelRateLimit, error) {
	var rls []model.LLMModelRateLimit
	if err := r.db.Find(&rls).Error; err != nil {
		return nil, err
	}
	return rls, nil
}

// SetLocked updates the locked state of a rate limit record by ID.
func (r *LLMRateLimitRepository) SetLocked(id uint, locked bool) error {
	return r.db.Model(&model.LLMModelRateLimit{}).Where("id = ?", id).Update("locked", locked).Error
}
