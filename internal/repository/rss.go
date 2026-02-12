package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type RSSRepository struct {
	db *gorm.DB
}

func NewRSSRepository(db *gorm.DB) *RSSRepository {
	return &RSSRepository{db: db}
}

func (r *RSSRepository) List(page, perPage int, category, keyword string) ([]model.RSSFeed, int64, error) {
	var feeds []model.RSSFeed
	var total int64

	query := r.db.Model(&model.RSSFeed{}).Preload("Tags")
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
	if err := query.Offset(offset).Limit(perPage).Order("id DESC").Find(&feeds).Error; err != nil {
		return nil, 0, err
	}

	return feeds, total, nil
}

func (r *RSSRepository) GetByID(id uint) (*model.RSSFeed, error) {
	var feed model.RSSFeed
	if err := r.db.Preload("Tags").First(&feed, id).Error; err != nil {
		return nil, err
	}
	return &feed, nil
}

func (r *RSSRepository) GetByURL(url string) (*model.RSSFeed, error) {
	var feed model.RSSFeed
	if err := r.db.Where("url = ?", url).First(&feed).Error; err != nil {
		return nil, err
	}
	return &feed, nil
}

func (r *RSSRepository) Create(feed *model.RSSFeed) error {
	return r.db.Create(feed).Error
}

func (r *RSSRepository) Update(feed *model.RSSFeed) error {
	return r.db.Save(feed).Error
}

func (r *RSSRepository) Delete(id uint) error {
	return r.db.Delete(&model.RSSFeed{}, id).Error
}

func (r *RSSRepository) UpdateTags(feed *model.RSSFeed, tags []model.Tag) error {
	return r.db.Model(feed).Association("Tags").Replace(tags)
}

func (r *RSSRepository) GetActive() ([]model.RSSFeed, error) {
	var feeds []model.RSSFeed
	if err := r.db.Where("is_active = ?", true).Preload("Tags").Find(&feeds).Error; err != nil {
		return nil, err
	}
	return feeds, nil
}
