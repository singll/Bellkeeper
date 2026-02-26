package service

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/urlutil"
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

// URLCheckResult holds the result of a single URL check
type URLCheckResult struct {
	Exists     bool   `json:"exists"`
	DocumentID string `json:"document_id,omitempty"`
	DatasetID  string `json:"dataset_id,omitempty"`
	Title      string `json:"title,omitempty"`
	StoredURL  string `json:"stored_url,omitempty"`
	MatchType  string `json:"match_type,omitempty"` // "exact", "normalized", "fuzzy"
}

// CheckURL checks if a URL exists in the local ArticleTag table with optional normalization and fuzzy matching
func (s *DatasetService) CheckURL(rawURL string, normalize bool, fuzzy bool) (*URLCheckResult, error) {
	// 1. Exact match
	ats, err := s.repo.FindArticleTagsByURL(rawURL)
	if err != nil {
		return nil, err
	}
	if len(ats) > 0 {
		return &URLCheckResult{
			Exists:     true,
			DocumentID: ats[0].DocumentID,
			DatasetID:  ats[0].DatasetID,
			Title:      ats[0].ArticleTitle,
			StoredURL:  ats[0].ArticleURL,
			MatchType:  "exact",
		}, nil
	}

	// 2. Normalized match
	if normalize {
		normalizedURL := urlutil.Normalize(rawURL)
		allArticles, err := s.repo.GetAllArticleURLs()
		if err != nil {
			return nil, err
		}
		for _, at := range allArticles {
			if urlutil.Normalize(at.ArticleURL) == normalizedURL {
				return &URLCheckResult{
					Exists:     true,
					DocumentID: at.DocumentID,
					DatasetID:  at.DatasetID,
					Title:      at.ArticleTitle,
					StoredURL:  at.ArticleURL,
					MatchType:  "normalized",
				}, nil
			}
		}
	}

	// 3. Fuzzy match
	if fuzzy {
		allArticles, err := s.repo.GetAllArticleURLs()
		if err != nil {
			return nil, err
		}
		for _, at := range allArticles {
			if urlutil.FuzzyMatch(rawURL, at.ArticleURL, 10) {
				return &URLCheckResult{
					Exists:     true,
					DocumentID: at.DocumentID,
					DatasetID:  at.DatasetID,
					Title:      at.ArticleTitle,
					StoredURL:  at.ArticleURL,
					MatchType:  "fuzzy",
				}, nil
			}
		}
	}

	return &URLCheckResult{Exists: false}, nil
}

// BatchCheckURLs checks multiple URLs at once
func (s *DatasetService) BatchCheckURLs(urls []string, normalize bool, fuzzy bool) (map[string]*URLCheckResult, error) {
	results := make(map[string]*URLCheckResult)

	// 1. Batch exact match
	ats, err := s.repo.FindArticleTagsByURLs(urls)
	if err != nil {
		return nil, err
	}
	exactMap := make(map[string]*model.ArticleTag)
	for i := range ats {
		if _, exists := exactMap[ats[i].ArticleURL]; !exists {
			exactMap[ats[i].ArticleURL] = &ats[i]
		}
	}
	for _, u := range urls {
		if at, ok := exactMap[u]; ok {
			results[u] = &URLCheckResult{
				Exists:     true,
				DocumentID: at.DocumentID,
				DatasetID:  at.DatasetID,
				Title:      at.ArticleTitle,
				StoredURL:  at.ArticleURL,
				MatchType:  "exact",
			}
		}
	}

	// 2. Normalized + fuzzy for unmatched URLs
	if normalize || fuzzy {
		var unmatched []string
		for _, u := range urls {
			if _, ok := results[u]; !ok {
				unmatched = append(unmatched, u)
			}
		}
		if len(unmatched) > 0 {
			allArticles, err := s.repo.GetAllArticleURLs()
			if err != nil {
				return nil, err
			}

			// Build normalized map
			var normalizedMap map[string]*model.ArticleTag
			if normalize {
				normalizedMap = make(map[string]*model.ArticleTag)
				for i := range allArticles {
					norm := urlutil.Normalize(allArticles[i].ArticleURL)
					if _, exists := normalizedMap[norm]; !exists {
						normalizedMap[norm] = &allArticles[i]
					}
				}
			}

			for _, u := range unmatched {
				// Normalized check
				if normalize && normalizedMap != nil {
					norm := urlutil.Normalize(u)
					if at, ok := normalizedMap[norm]; ok {
						results[u] = &URLCheckResult{
							Exists:     true,
							DocumentID: at.DocumentID,
							DatasetID:  at.DatasetID,
							Title:      at.ArticleTitle,
							StoredURL:  at.ArticleURL,
							MatchType:  "normalized",
						}
						continue
					}
				}
				// Fuzzy check
				if fuzzy {
					for i := range allArticles {
						if urlutil.FuzzyMatch(u, allArticles[i].ArticleURL, 10) {
							results[u] = &URLCheckResult{
								Exists:     true,
								DocumentID: allArticles[i].DocumentID,
								DatasetID:  allArticles[i].DatasetID,
								Title:      allArticles[i].ArticleTitle,
								StoredURL:  allArticles[i].ArticleURL,
								MatchType:  "fuzzy",
							}
							break
						}
					}
				}
				// Not found
				if _, ok := results[u]; !ok {
					results[u] = &URLCheckResult{Exists: false}
				}
			}
		}
	} else {
		// Mark remaining as not found
		for _, u := range urls {
			if _, ok := results[u]; !ok {
				results[u] = &URLCheckResult{Exists: false}
			}
		}
	}

	return results, nil
}
