package service

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type DatasetService struct {
	repo    *repository.DatasetMappingRepository
	tagRepo *repository.TagRepository
}

func NewDatasetService(repo *repository.DatasetMappingRepository, tagRepo *repository.TagRepository) *DatasetService {
	return &DatasetService{repo: repo, tagRepo: tagRepo}
}

func (s *DatasetService) List(page, perPage int) ([]model.DatasetMapping, int64, error) {
	return s.repo.List(page, perPage)
}

func (s *DatasetService) GetByID(id uint) (*model.DatasetMapping, error) {
	return s.repo.GetByID(id)
}

func (s *DatasetService) GetByName(name string) (*model.DatasetMapping, error) {
	return s.repo.GetByName(name)
}

func (s *DatasetService) GetDefault() (*model.DatasetMapping, error) {
	return s.repo.GetDefault()
}

func (s *DatasetService) Create(mapping *model.DatasetMapping, tagIDs []uint) error {
	if err := s.repo.Create(mapping); err != nil {
		return err
	}

	if len(tagIDs) > 0 {
		tags, err := s.tagRepo.GetByIDs(tagIDs)
		if err != nil {
			return err
		}
		return s.repo.UpdateTags(mapping, tags)
	}

	return nil
}

func (s *DatasetService) Update(mapping *model.DatasetMapping, tagIDs []uint) error {
	if err := s.repo.Update(mapping); err != nil {
		return err
	}

	tags, err := s.tagRepo.GetByIDs(tagIDs)
	if err != nil {
		return err
	}
	return s.repo.UpdateTags(mapping, tags)
}

func (s *DatasetService) Delete(id uint) error {
	return s.repo.Delete(id)
}

// GetByTagIDs finds dataset mappings that match any of the given tag IDs
func (s *DatasetService) GetByTagIDs(tagIDs []uint) ([]model.DatasetMapping, error) {
	return s.repo.GetByTagIDs(tagIDs)
}

// --- Batch C: 高级端点 ---

func (s *DatasetService) GetAll() ([]model.DatasetMapping, error) {
	return s.repo.GetAll()
}

// RecommendByTags finds the best dataset for the given tags, with fallback to category and default
func (s *DatasetService) RecommendByTags(tagNames []string, category string) (*model.DatasetMapping, string, error) {
	// 1. Try by tags
	if len(tagNames) > 0 {
		tags, err := s.tagRepo.GetByNames(tagNames)
		if err == nil && len(tags) > 0 {
			var tagIDs []uint
			for _, t := range tags {
				tagIDs = append(tagIDs, t.ID)
			}
			mappings, err := s.repo.GetByTagIDs(tagIDs)
			if err == nil && len(mappings) > 0 {
				return &mappings[0], "tag", nil
			}
		}
	}

	// 2. Try by category (name)
	if category != "" {
		mapping, err := s.repo.GetByName(category)
		if err == nil {
			return mapping, "category", nil
		}
	}

	// 3. Fallback to default
	mapping, err := s.repo.GetDefault()
	if err != nil {
		return nil, "", err
	}
	return mapping, "default", nil
}

// AddArticleTags creates article-tag associations
func (s *DatasetService) AddArticleTags(documentID, datasetID string, tagIDs []uint, title, url string) ([]model.ArticleTag, error) {
	var created []model.ArticleTag
	for _, tagID := range tagIDs {
		at := &model.ArticleTag{
			DocumentID:   documentID,
			DatasetID:    datasetID,
			TagID:        tagID,
			ArticleTitle: title,
			ArticleURL:   url,
		}
		if err := s.repo.CreateArticleTag(at); err != nil {
			return nil, err
		}
		created = append(created, *at)
	}
	return created, nil
}

// GetArticleTags returns tags for a given document
func (s *DatasetService) GetArticleTags(documentID string) ([]model.ArticleTag, error) {
	return s.repo.GetArticleTagsByDocumentID(documentID)
}

// GetArticlesByTag returns paginated articles for a given tag
func (s *DatasetService) GetArticlesByTag(tagID uint, page, perPage int) ([]model.ArticleTag, int64, error) {
	return s.repo.GetArticlesByTagID(tagID, page, perPage)
}

// DeleteArticleTagsByDocumentIDs cleans up article-tag associations
func (s *DatasetService) DeleteArticleTagsByDocumentIDs(documentIDs []string) error {
	return s.repo.DeleteArticleTagsByDocumentIDs(documentIDs)
}
