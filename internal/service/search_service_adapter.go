package service

import (
	"context"

	"github.com/singll/bellkeeper/internal/matrix/command"
)

// SearchServiceAdapter 将 FileSearchService 适配为 command.SearchHandler 接口
// 用于在 command 包中使用 FileSearchService 而不产生循环依赖
type SearchServiceAdapter struct {
	svc *FileSearchService
}

// NewSearchServiceAdapter 创建适配器
func NewSearchServiceAdapter(svc *FileSearchService) *SearchServiceAdapter {
	return &SearchServiceAdapter{svc: svc}
}

// Search 实现 command.SearchHandler 接口
func (a *SearchServiceAdapter) Search(ctx context.Context, query string, limit int) ([]command.SearchHit, error) {
	req := FileSearchRequest{
		Query: query,
		Limit: limit,
	}

	result, err := a.svc.Search(ctx, req)
	if err != nil {
		return nil, err
	}

	hits := make([]command.SearchHit, 0, len(result.Files))
	for _, f := range result.Files {
		hits = append(hits, command.SearchHit{
			Title:    f.Title,
			FilePath: f.FilePath,
			Layer:    f.Layer,
			Snippets: f.Snippets,
		})
	}

	return hits, nil
}
