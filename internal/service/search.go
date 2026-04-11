package service

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// SearchService handles cross-entity search
type SearchService struct {
	tagRepo       *repository.TagRepository
	articleTagRepo *repository.ArticleTagRepository
	rssRepo       *repository.RSSRepository
}

// SearchResult contains search results from all entities
type SearchResult struct {
	Tags       []model.Tag        `json:"tags"`
	Documents  []model.ArticleTag `json:"documents"`
	RSSFeeds   []model.RSSFeed   `json:"rss_feeds"`
	TotalCount int64              `json:"total_count"`
}

// NewSearchService creates a new SearchService
func NewSearchService(
	tagRepo *repository.TagRepository,
	articleTagRepo *repository.ArticleTagRepository,
	rssRepo *repository.RSSRepository,
) *SearchService {
	return &SearchService{
		tagRepo:       tagRepo,
		articleTagRepo: articleTagRepo,
		rssRepo:       rssRepo,
	}
}

// Search performs cross-entity search
// scope: all, tags, documents, rss
func (s *SearchService) Search(query string, scope string, limit int) (*SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	result := &SearchResult{
		Tags:       []model.Tag{},
		Documents:  []model.ArticleTag{},
		RSSFeeds:   []model.RSSFeed{},
		TotalCount: 0,
	}

	keyword := "%" + query + "%"

	// Search tags
	if scope == "all" || scope == "tags" {
		tags, err := s.tagRepo.Search(keyword, limit)
		if err == nil {
			result.Tags = tags
			result.TotalCount += int64(len(tags))
		}
	}

	// Search documents
	if scope == "all" || scope == "documents" {
		docs, _, err := s.articleTagRepo.ListWithFilter(repository.ListArticleTagOpts{
			Keyword: query,
			Page:    1,
			PerPage: limit,
		})
		if err == nil {
			result.Documents = docs
			result.TotalCount += int64(len(docs))
		}
	}

	// Search RSS feeds
	if scope == "all" || scope == "rss" {
		feeds, err := s.rssRepo.Search(keyword, limit)
		if err == nil {
			result.RSSFeeds = feeds
			result.TotalCount += int64(len(feeds))
		}
	}

	return result, nil
}
