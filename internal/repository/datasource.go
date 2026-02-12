package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type DataSourceRepository struct {
	db *gorm.DB
}

func NewDataSourceRepository(db *gorm.DB) *DataSourceRepository {
	return &DataSourceRepository{db: db}
}

func (r *DataSourceRepository) List(page, perPage int, category, keyword string) ([]model.DataSource, int64, error) {
	var sources []model.DataSource
	var total int64

	query := r.db.Model(&model.DataSource{}).Preload("Tags")
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if keyword != "" {
		query = query.Where("name ILIKE ? OR url ILIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("id DESC").Find(&sources).Error; err != nil {
		return nil, 0, err
	}

	return sources, total, nil
}

func (r *DataSourceRepository) GetByID(id uint) (*model.DataSource, error) {
	var source model.DataSource
	if err := r.db.Preload("Tags").First(&source, id).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func (r *DataSourceRepository) Create(source *model.DataSource) error {
	return r.db.Create(source).Error
}

func (r *DataSourceRepository) Update(source *model.DataSource) error {
	return r.db.Save(source).Error
}

func (r *DataSourceRepository) Delete(id uint) error {
	return r.db.Delete(&model.DataSource{}, id).Error
}

func (r *DataSourceRepository) UpdateTags(source *model.DataSource, tags []model.Tag) error {
	return r.db.Model(source).Association("Tags").Replace(tags)
}
