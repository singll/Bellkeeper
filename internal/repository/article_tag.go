package repository

import (
	"github.com/singll/bellkeeper/internal/model"

	"gorm.io/gorm"
)

// ArticleTagRepository handles database operations for ArticleTag
type ArticleTagRepository struct {
	db *gorm.DB
}

// NewArticleTagRepository creates a new ArticleTagRepository
func NewArticleTagRepository(db *gorm.DB) *ArticleTagRepository {
	return &ArticleTagRepository{db: db}
}

// Create creates a new article tag record
func (r *ArticleTagRepository) Create(article *model.ArticleTag) error {
	return r.db.Create(article).Error
}

// GetByID retrieves an article tag by ID
func (r *ArticleTagRepository) GetByID(id uint) (*model.ArticleTag, error) {
	var article model.ArticleTag
	err := r.db.First(&article, id).Error
	if err != nil {
		return nil, err
	}
	return &article, nil
}

// GetByURL retrieves an article tag by URL
func (r *ArticleTagRepository) GetByURL(url string) (*model.ArticleTag, error) {
	var article model.ArticleTag
	err := r.db.Where("article_url = ?", url).First(&article).Error
	if err != nil {
		return nil, err
	}
	return &article, nil
}

// GetByContentHash retrieves an article tag by content hash
func (r *ArticleTagRepository) GetByContentHash(hash string) (*model.ArticleTag, error) {
	var article model.ArticleTag
	err := r.db.Where("content_hash = ?", hash).First(&article).Error
	if err != nil {
		return nil, err
	}
	return &article, nil
}

// GetByFilePath retrieves an article tag by file path
func (r *ArticleTagRepository) GetByFilePath(path string) (*model.ArticleTag, error) {
	var article model.ArticleTag
	err := r.db.Where("file_path = ?", path).First(&article).Error
	if err != nil {
		return nil, err
	}
	return &article, nil
}

// UpdateIndexStatus updates the index status of an article
func (r *ArticleTagRepository) UpdateIndexStatus(id uint, status string) error {
	return r.db.Model(&model.ArticleTag{}).Where("id = ?", id).Update("index_status", status).Error
}

// UpdateIngestStatus updates the ingest status of an article
func (r *ArticleTagRepository) UpdateIngestStatus(id uint, status string) error {
	return r.db.Model(&model.ArticleTag{}).Where("id = ?", id).Update("ingest_status", status).Error
}

// ListPendingIndex retrieves articles with pending index status
func (r *ArticleTagRepository) ListPendingIndex(limit int) ([]model.ArticleTag, error) {
	var articles []model.ArticleTag
	err := r.db.Where("index_status = ?", "pending").
		Order("created_at ASC").
		Limit(limit).
		Find(&articles).Error
	return articles, err
}

// ListByLayer retrieves articles by layer
func (r *ArticleTagRepository) ListByLayer(layer string, limit, offset int) ([]model.ArticleTag, error) {
	var articles []model.ArticleTag
	err := r.db.Where("layer = ?", layer).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&articles).Error
	return articles, err
}

// ListByStatus retrieves articles by ingest status
func (r *ArticleTagRepository) ListByStatus(status string, limit, offset int) ([]model.ArticleTag, error) {
	var articles []model.ArticleTag
	err := r.db.Where("ingest_status = ?", status).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&articles).Error
	return articles, err
}

// Count returns the total count of articles
func (r *ArticleTagRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&model.ArticleTag{}).Count(&count).Error
	return count, err
}

// CountByLayer returns the count of articles by layer
func (r *ArticleTagRepository) CountByLayer(layer string) (int64, error) {
	var count int64
	err := r.db.Model(&model.ArticleTag{}).Where("layer = ?", layer).Count(&count).Error
	return count, err
}

// CountByStatus returns the count of articles by status
func (r *ArticleTagRepository) CountByStatus(status string) (int64, error) {
	var count int64
	err := r.db.Model(&model.ArticleTag{}).Where("ingest_status = ?", status).Count(&count).Error
	return count, err
}

// Delete soft deletes an article tag
func (r *ArticleTagRepository) Delete(id uint) error {
	return r.db.Delete(&model.ArticleTag{}, id).Error
}

// Update updates an article tag
func (r *ArticleTagRepository) Update(article *model.ArticleTag) error {
	return r.db.Save(article).Error
}

// ListArticleTagOpts contains filter options for listing article tags
type ListArticleTagOpts struct {
	Layer   string
	Status  string
	Keyword string
	Page    int
	PerPage int
}

// ListWithFilter retrieves article tags with filtering and pagination
func (r *ArticleTagRepository) ListWithFilter(opts ListArticleTagOpts) ([]model.ArticleTag, int64, error) {
	var articles []model.ArticleTag
	var total int64

	query := r.db.Model(&model.ArticleTag{})

	// Apply filters
	if opts.Layer != "" {
		query = query.Where("layer = ?", opts.Layer)
	}
	if opts.Status != "" {
		query = query.Where("ingest_status = ?", opts.Status)
	}
	if opts.Keyword != "" {
		keyword := "%" + opts.Keyword + "%"
		query = query.Where("article_title LIKE ? OR article_url LIKE ? OR source_domain LIKE ?",
			keyword, keyword, keyword)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PerPage < 1 {
		opts.PerPage = 20
	}
	offset := (opts.Page - 1) * opts.PerPage

	// Query with pagination
	err := query.
		Order("created_at DESC").
		Limit(opts.PerPage).
		Offset(offset).
		Find(&articles).Error

	return articles, total, err
}

// GetByIDWithPreload retrieves an article tag by ID with preloaded associations
func (r *ArticleTagRepository) GetByIDWithPreload(id uint) (*model.ArticleTag, error) {
	var article model.ArticleTag
	err := r.db.Preload("Tag").First(&article, id).Error
	if err != nil {
		return nil, err
	}
	return &article, nil
}
