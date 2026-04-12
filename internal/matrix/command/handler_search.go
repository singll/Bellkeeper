package command

import (
	"context"
	"fmt"
	"strings"
)

// SearchHandler 定义搜索功能的接口
// 用于解耦 command 包和 service 包的循环依赖
type SearchHandler interface {
	Search(ctx context.Context, query string, limit int) ([]SearchHit, error)
}

// SearchHit 搜索命中结果
type SearchHit struct {
	Title    string
	FilePath string
	Layer    string
	Snippets []string
}

// MatrixSearchHandler Matrix 搜索命令处理器
type MatrixSearchHandler struct {
	BaseHandler
	searchHandler SearchHandler
}

// NewMatrixSearchHandler 创建搜索处理器
func NewMatrixSearchHandler(searchHandler SearchHandler) *MatrixSearchHandler {
	return &MatrixSearchHandler{
		BaseHandler: BaseHandler{
			name:        "search",
			description: "搜索知识库",
			usage:       "<关键词>",
		},
		searchHandler: searchHandler,
	}
}

// Handle 处理搜索命令
func (h *MatrixSearchHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	query := cmdCtx.Command.Args
	if query == "" {
		return &Response{
			Success: true,
			Message: "请输入搜索关键词，例如: `!搜 渗透测试`",
			IsHTML:  false,
		}, nil
	}

	hits, err := h.searchHandler.Search(ctx, query, 5)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 搜索失败: %v", err),
		}, nil
	}

	msg := h.formatResponse(query, hits)
	return &Response{
		Success: true,
		Message: msg,
		IsHTML:  false,
	}, nil
}

// formatResponse 格式化响应
func (h *MatrixSearchHandler) formatResponse(query string, hits []SearchHit) string {
	var sb strings.Builder

	if len(hits) == 0 {
		sb.WriteString(fmt.Sprintf("🔍 搜索: %s (未找到相关文件)", query))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("🔍 搜索: %s (找到 %d 个相关文件)\n\n", query, len(hits)))

	for i, hit := range hits {
		sb.WriteString(fmt.Sprintf("%d. %s — %s\n", i+1, hit.Title, hit.Layer))
		if len(hit.Snippets) > 0 {
			sb.WriteString(fmt.Sprintf("   %s\n", hit.Snippets[0]))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
