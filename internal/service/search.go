package service

import (
	"fmt"
	"strings"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type tagSearchRepository interface {
	Search(keyword string, limit int) ([]model.Tag, error)
}

type articleSearchRepository interface {
	ListWithFilter(opts repository.ListArticleTagOpts) ([]model.ArticleTag, int64, error)
}

type rssSearchRepository interface {
	Search(keyword string, limit int) ([]model.RSSFeed, error)
}

// SearchService handles cross-entity search
type SearchService struct {
	tagRepo        tagSearchRepository
	articleTagRepo articleSearchRepository
	rssRepo        rssSearchRepository
}

// SearchResult contains search results from all entities
type SearchResult struct {
	Tags       []model.Tag        `json:"tags"`
	Documents  []model.ArticleTag `json:"documents"`
	RSSFeeds   []model.RSSFeed    `json:"rss_feeds"`
	TotalCount int64              `json:"total_count"`
}

// NewSearchService creates a new SearchService
func NewSearchService(
	tagRepo tagSearchRepository,
	articleTagRepo articleSearchRepository,
	rssRepo rssSearchRepository,
) *SearchService {
	return &SearchService{
		tagRepo:        tagRepo,
		articleTagRepo: articleTagRepo,
		rssRepo:        rssRepo,
	}
}

// Search performs cross-entity search
// scope: all, tags, documents, rss
func (s *SearchService) Search(query string, scope string, limit int) (*SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	if scope == "" {
		scope = "all"
	}
	query = strings.TrimSpace(query)

	result := &SearchResult{
		Tags:       []model.Tag{},
		Documents:  []model.ArticleTag{},
		RSSFeeds:   []model.RSSFeed{},
		TotalCount: 0,
	}

	keyword := "%" + query + "%"

	// Search tags
	if scope == "all" || scope == "tags" {
		if s.tagRepo == nil {
			return nil, fmt.Errorf("tag repository not configured")
		}
		tags, err := s.tagRepo.Search(keyword, limit)
		if err != nil {
			return nil, fmt.Errorf("search tags: %w", err)
		}
		result.Tags = tags
		result.TotalCount += int64(len(tags))
	}

	// Search documents
	if scope == "all" || scope == "documents" {
		if s.articleTagRepo == nil {
			return nil, fmt.Errorf("article repository not configured")
		}
		docs, _, err := s.articleTagRepo.ListWithFilter(repository.ListArticleTagOpts{
			Keyword: query,
			Page:    1,
			PerPage: limit,
		})
		if err != nil {
			return nil, fmt.Errorf("search documents: %w", err)
		}
		result.Documents = docs
		result.TotalCount += int64(len(docs))
	}

	// Search RSS feeds
	if scope == "all" || scope == "rss" {
		if s.rssRepo == nil {
			return nil, fmt.Errorf("rss repository not configured")
		}
		feeds, err := s.rssRepo.Search(keyword, limit)
		if err != nil {
			return nil, fmt.Errorf("search rss: %w", err)
		}
		result.RSSFeeds = feeds
		result.TotalCount += int64(len(feeds))
	}

	if scope != "all" && scope != "tags" && scope != "documents" && scope != "rss" {
		return nil, fmt.Errorf("invalid search scope: %s", scope)
	}

	return result, nil
}
