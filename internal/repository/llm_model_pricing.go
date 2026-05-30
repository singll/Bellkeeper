package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type LLMModelPricingRepository struct {
	db *gorm.DB
}

func NewLLMModelPricingRepository(db *gorm.DB) *LLMModelPricingRepository {
	return &LLMModelPricingRepository{db: db}
}

func (r *LLMModelPricingRepository) List() ([]model.LLMModelPricing, error) {
	var pricing []model.LLMModelPricing
	if err := r.db.Order("channel_name ASC, model ASC").Find(&pricing).Error; err != nil {
		return nil, err
	}
	return pricing, nil
}

func (r *LLMModelPricingRepository) Get(id uint) (*model.LLMModelPricing, error) {
	var p model.LLMModelPricing
	if err := r.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *LLMModelPricingRepository) GetByChannelAndModel(channelName, modelName string) (*model.LLMModelPricing, error) {
	var p model.LLMModelPricing
	if err := r.db.Where("channel_name = ? AND model = ?", channelName, modelName).First(&p).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *LLMModelPricingRepository) Create(p *model.LLMModelPricing) error {
	return r.db.Create(p).Error
}

func (r *LLMModelPricingRepository) Update(p *model.LLMModelPricing) error {
	return r.db.Save(p).Error
}

func (r *LLMModelPricingRepository) Delete(id uint) error {
	return r.db.Delete(&model.LLMModelPricing{}, id).Error
}

func (r *LLMModelPricingRepository) Count() (int64, error) {
	var count int64
	if err := r.db.Model(&model.LLMModelPricing{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
