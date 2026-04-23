package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type LogSourceRepository struct {
	db *gorm.DB
}

func NewLogSourceRepository(db *gorm.DB) *LogSourceRepository {
	return &LogSourceRepository{db: db}
}

func (r *LogSourceRepository) Create(source *model.LogSource) error {
	return r.db.Create(source).Error
}

func (r *LogSourceRepository) GetByID(id uint) (*model.LogSource, error) {
	var source model.LogSource
	if err := r.db.First(&source, id).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func (r *LogSourceRepository) GetByName(name string) (*model.LogSource, error) {
	var source model.LogSource
	if err := r.db.Where("name = ?", name).First(&source).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func (r *LogSourceRepository) GetByAPIKey(apiKey string) (*model.LogSource, error) {
	var source model.LogSource
	if err := r.db.Where("api_key = ? AND is_active = ?", apiKey, true).First(&source).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func (r *LogSourceRepository) List() ([]model.LogSource, error) {
	var sources []model.LogSource
	if err := r.db.Order("id ASC").Find(&sources).Error; err != nil {
		return nil, err
	}
	return sources, nil
}

func (r *LogSourceRepository) Update(source *model.LogSource) error {
	return r.db.Save(source).Error
}

func (r *LogSourceRepository) Delete(id uint) error {
	return r.db.Delete(&model.LogSource{}, id).Error
}