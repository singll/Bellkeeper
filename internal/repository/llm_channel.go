package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type LLMChannelRepository struct {
	db *gorm.DB
}

func NewLLMChannelRepository(db *gorm.DB) *LLMChannelRepository {
	return &LLMChannelRepository{db: db}
}

func (r *LLMChannelRepository) List() ([]model.LLMChannel, error) {
	var channels []model.LLMChannel
	if err := r.db.Order("priority ASC, name ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

func (r *LLMChannelRepository) ListEnabled() ([]model.LLMChannel, error) {
	var channels []model.LLMChannel
	if err := r.db.Where("is_enabled = ?", true).Order("priority ASC, name ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

func (r *LLMChannelRepository) Get(id uint) (*model.LLMChannel, error) {
	var ch model.LLMChannel
	if err := r.db.First(&ch, id).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

func (r *LLMChannelRepository) GetByName(name string) (*model.LLMChannel, error) {
	var ch model.LLMChannel
	if err := r.db.Where("name = ?", name).First(&ch).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

func (r *LLMChannelRepository) Create(ch *model.LLMChannel) error {
	return r.db.Create(ch).Error
}

func (r *LLMChannelRepository) Update(ch *model.LLMChannel) error {
	return r.db.Save(ch).Error
}

func (r *LLMChannelRepository) Delete(id uint) error {
	return r.db.Delete(&model.LLMChannel{}, id).Error
}
