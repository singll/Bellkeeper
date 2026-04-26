package repository

import (
	"time"

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
	if err := r.db.Where("is_active = ? AND is_paused = ?", true, false).Preload("Tags").Find(&feeds).Error; err != nil {
		return nil, err
	}
	return feeds, nil
}

// GetActiveIncludingPaused returns all active feeds including paused ones (for health dashboards)
func (r *RSSRepository) GetActiveIncludingPaused() ([]model.RSSFeed, error) {
	var feeds []model.RSSFeed
	if err := r.db.Where("is_active = ?", true).Preload("Tags").Find(&feeds).Error; err != nil {
		return nil, err
	}
	return feeds, nil
}

// BatchUpdatePaused 批量更新源的暂停状态，返回受影响行数
func (r *RSSRepository) BatchUpdatePaused(ids []uint, paused bool) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	now := time.Now()
	updates := map[string]interface{}{
		"is_paused":            paused,
		"consecutive_failures": 0,
		"last_failure_reason":  "",
	}
	if paused {
		updates["paused_at"] = now
	} else {
		updates["paused_at"] = nil
		updates["health_score"] = 30
	}
	result := r.db.Model(&model.RSSFeed{}).Where("id IN ?", ids).Updates(updates)
	return result.RowsAffected, result.Error
}

// UpdateAllPaused 更新所有活跃源的暂停状态，返回受影响行数
func (r *RSSRepository) UpdateAllPaused(paused bool) (int64, error) {
	now := time.Now()
	updates := map[string]interface{}{
		"is_paused":            paused,
		"consecutive_failures": 0,
		"last_failure_reason":  "",
	}
	if paused {
		updates["paused_at"] = now
	} else {
		updates["paused_at"] = nil
		updates["health_score"] = 30
	}
	result := r.db.Model(&model.RSSFeed{}).Where("is_active = ?", true).Updates(updates)
	return result.RowsAffected, result.Error
}

// Search searches RSS feeds by name or URL
func (r *RSSRepository) Search(keyword string, limit int) ([]model.RSSFeed, error) {
	var feeds []model.RSSFeed
	if limit <= 0 {
		limit = 10
	}
	if err := r.db.Where("name ILIKE ? OR url ILIKE ?", keyword, keyword).Preload("Tags").Order("name ASC").Limit(limit).Find(&feeds).Error; err != nil {
		return nil, err
	}
	return feeds, nil
}
