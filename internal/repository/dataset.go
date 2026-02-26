package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type DatasetMappingRepository struct {
	db *gorm.DB
}

func NewDatasetMappingRepository(db *gorm.DB) *DatasetMappingRepository {
	return &DatasetMappingRepository{db: db}
}

func (r *DatasetMappingRepository) List(page, perPage int) ([]model.DatasetMapping, int64, error) {
	var mappings []model.DatasetMapping
	var total int64

	query := r.db.Model(&model.DatasetMapping{}).Preload("Tags")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("id DESC").Find(&mappings).Error; err != nil {
		return nil, 0, err
	}

	return mappings, total, nil
}

func (r *DatasetMappingRepository) GetByID(id uint) (*model.DatasetMapping, error) {
	var mapping model.DatasetMapping
	if err := r.db.Preload("Tags").First(&mapping, id).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}

func (r *DatasetMappingRepository) GetByName(name string) (*model.DatasetMapping, error) {
	var mapping model.DatasetMapping
	if err := r.db.Preload("Tags").Where("name = ?", name).First(&mapping).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}

func (r *DatasetMappingRepository) GetDefault() (*model.DatasetMapping, error) {
	var mapping model.DatasetMapping
	if err := r.db.Preload("Tags").Where("is_default = ?", true).First(&mapping).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}

func (r *DatasetMappingRepository) Create(mapping *model.DatasetMapping) error {
	return r.db.Create(mapping).Error
}

func (r *DatasetMappingRepository) Update(mapping *model.DatasetMapping) error {
	return r.db.Save(mapping).Error
}

func (r *DatasetMappingRepository) Delete(id uint) error {
	return r.db.Delete(&model.DatasetMapping{}, id).Error
}

func (r *DatasetMappingRepository) UpdateTags(mapping *model.DatasetMapping, tags []model.Tag) error {
	return r.db.Model(mapping).Association("Tags").Replace(tags)
}

func (r *DatasetMappingRepository) GetByTagIDs(tagIDs []uint) ([]model.DatasetMapping, error) {
	var mappings []model.DatasetMapping
	if err := r.db.Preload("Tags").
		Joins("JOIN dataset_mapping_tags ON dataset_mappings.id = dataset_mapping_tags.mapping_id").
		Where("dataset_mapping_tags.tag_id IN ?", tagIDs).
		Group("dataset_mappings.id").
		Find(&mappings).Error; err != nil {
		return nil, err
	}
	return mappings, nil
}

// ArticleTag operations
func (r *DatasetMappingRepository) CreateArticleTag(at *model.ArticleTag) error {
	return r.db.Create(at).Error
}

func (r *DatasetMappingRepository) GetArticleTagByURL(url string) (*model.ArticleTag, error) {
	var at model.ArticleTag
	if err := r.db.Where("article_url = ?", url).First(&at).Error; err != nil {
		return nil, err
	}
	return &at, nil
}

func (r *DatasetMappingRepository) ArticleURLExists(url string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.ArticleTag{}).Where("article_url = ?", url).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// --- Batch C: 高级端点 ---

func (r *DatasetMappingRepository) GetAll() ([]model.DatasetMapping, error) {
	var mappings []model.DatasetMapping
	if err := r.db.Preload("Tags").Where("is_active = ?", true).Order("name ASC").Find(&mappings).Error; err != nil {
		return nil, err
	}
	return mappings, nil
}

func (r *DatasetMappingRepository) GetArticleTagsByDocumentID(documentID string) ([]model.ArticleTag, error) {
	var ats []model.ArticleTag
	if err := r.db.Preload("Tag").Where("document_id = ?", documentID).Find(&ats).Error; err != nil {
		return nil, err
	}
	return ats, nil
}

func (r *DatasetMappingRepository) GetArticlesByTagID(tagID uint, page, perPage int) ([]model.ArticleTag, int64, error) {
	var ats []model.ArticleTag
	var total int64

	query := r.db.Model(&model.ArticleTag{}).Where("tag_id = ?", tagID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Preload("Tag").Offset(offset).Limit(perPage).Order("created_at DESC").Find(&ats).Error; err != nil {
		return nil, 0, err
	}

	return ats, total, nil
}

func (r *DatasetMappingRepository) DeleteArticleTagsByDocumentIDs(documentIDs []string) error {
	return r.db.Where("document_id IN ?", documentIDs).Delete(&model.ArticleTag{}).Error
}

// FindArticleTagsByURL finds article tags matching the exact URL
func (r *DatasetMappingRepository) FindArticleTagsByURL(url string) ([]model.ArticleTag, error) {
	var ats []model.ArticleTag
	if err := r.db.Preload("Tag").Where("article_url = ?", url).Find(&ats).Error; err != nil {
		return nil, err
	}
	return ats, nil
}

// FindArticleTagsByURLs finds article tags matching any of the given URLs
func (r *DatasetMappingRepository) FindArticleTagsByURLs(urls []string) ([]model.ArticleTag, error) {
	var ats []model.ArticleTag
	if err := r.db.Preload("Tag").Where("article_url IN ?", urls).Find(&ats).Error; err != nil {
		return nil, err
	}
	return ats, nil
}

// GetAllArticleURLs returns all article URLs (for normalized matching)
func (r *DatasetMappingRepository) GetAllArticleURLs() ([]model.ArticleTag, error) {
	var ats []model.ArticleTag
	if err := r.db.Select("id, document_id, dataset_id, article_url, article_title").Find(&ats).Error; err != nil {
		return nil, err
	}
	return ats, nil
}
