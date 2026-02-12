package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type SettingRepository struct {
	db *gorm.DB
}

func NewSettingRepository(db *gorm.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

func (r *SettingRepository) List(category string) ([]model.Setting, error) {
	var settings []model.Setting
	query := r.db.Model(&model.Setting{})
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if err := query.Order("key ASC").Find(&settings).Error; err != nil {
		return nil, err
	}
	return settings, nil
}

func (r *SettingRepository) GetByKey(key string) (*model.Setting, error) {
	var setting model.Setting
	if err := r.db.Where("key = ?", key).First(&setting).Error; err != nil {
		return nil, err
	}
	return &setting, nil
}

func (r *SettingRepository) Set(key, value, valueType, category, description string, isSecret bool) error {
	setting := model.Setting{
		Key:         key,
		Value:       value,
		ValueType:   valueType,
		Category:    category,
		Description: description,
		IsSecret:    isSecret,
	}

	return r.db.Where("key = ?", key).Assign(setting).FirstOrCreate(&setting).Error
}

func (r *SettingRepository) Delete(key string) error {
	return r.db.Where("key = ?", key).Delete(&model.Setting{}).Error
}
