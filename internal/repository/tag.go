package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type TagRepository struct {
	db *gorm.DB
}

func NewTagRepository(db *gorm.DB) *TagRepository {
	return &TagRepository{db: db}
}

func (r *TagRepository) List(page, perPage int, keyword string) ([]model.Tag, int64, error) {
	var tags []model.Tag
	var total int64

	query := r.db.Model(&model.Tag{})
	if keyword != "" {
		query = query.Where("name ILIKE ?", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("id DESC").Find(&tags).Error; err != nil {
		return nil, 0, err
	}

	return tags, total, nil
}

func (r *TagRepository) GetByID(id uint) (*model.Tag, error) {
	var tag model.Tag
	if err := r.db.First(&tag, id).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

func (r *TagRepository) GetByName(name string) (*model.Tag, error) {
	var tag model.Tag
	if err := r.db.Where("name = ?", name).First(&tag).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

func (r *TagRepository) Create(tag *model.Tag) error {
	return r.db.Create(tag).Error
}

// FindOrCreate gets a tag by name, creating it if it doesn't exist.
// Handles soft-deleted tags by restoring them and concurrent creation races.
func (r *TagRepository) FindOrCreate(name, color string) (*model.Tag, error) {
	// 1. Try normal lookup (excludes soft-deleted)
	var tag model.Tag
	if err := r.db.Where("name = ?", name).First(&tag).Error; err == nil {
		return &tag, nil
	}

	// 2. Check for soft-deleted tag and restore it
	var softDeleted model.Tag
	if err := r.db.Unscoped().Where("name = ?", name).First(&softDeleted).Error; err == nil {
		// Restore: clear DeletedAt, update color
		r.db.Unscoped().Model(&softDeleted).Updates(map[string]interface{}{
			"deleted_at": nil,
			"color":      color,
		})
		return &softDeleted, nil
	}

	// 3. Create new tag
	tag = model.Tag{Name: name, Color: color}
	if err := r.db.Create(&tag).Error; err != nil {
		// 4. Race condition: another request created it concurrently
		if err2 := r.db.Where("name = ?", name).First(&tag).Error; err2 == nil {
			return &tag, nil
		}
		return nil, err
	}
	return &tag, nil
}

func (r *TagRepository) Update(tag *model.Tag) error {
	return r.db.Save(tag).Error
}

func (r *TagRepository) Delete(id uint) error {
	return r.db.Delete(&model.Tag{}, id).Error
}

func (r *TagRepository) GetByIDs(ids []uint) ([]model.Tag, error) {
	var tags []model.Tag
	if err := r.db.Where("id IN ?", ids).Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func (r *TagRepository) GetAll() ([]model.Tag, error) {
	var tags []model.Tag
	if err := r.db.Order("name ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func (r *TagRepository) GetByNames(names []string) ([]model.Tag, error) {
	var tags []model.Tag
	if err := r.db.Where("name IN ?", names).Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func (r *TagRepository) MatchByKeyword(keyword string) ([]model.Tag, error) {
	var tags []model.Tag
	if err := r.db.Where("name ILIKE ?", "%"+keyword+"%").Order("name ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// Search searches tags by name or description
func (r *TagRepository) Search(keyword string, limit int) ([]model.Tag, error) {
	var tags []model.Tag
	if limit <= 0 {
		limit = 10
	}
	if err := r.db.Where("name ILIKE ? OR description ILIKE ?", keyword, keyword).Order("name ASC").Limit(limit).Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}
