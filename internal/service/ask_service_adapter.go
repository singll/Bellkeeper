package service

import (
	"context"

	"github.com/singll/bellkeeper/internal/matrix/command"
)

// AskServiceAdapter 将 AskService 适配为 command.AskHandler 接口
// 用于在 command 包中使用 AskService而不产生循环依赖
type AskServiceAdapter struct {
	svc *AskService
}

// NewAskServiceAdapter 创建适配器
func NewAskServiceAdapter(svc *AskService) *AskServiceAdapter {
	return &AskServiceAdapter{svc: svc}
}

// Ask 实现 command.AskHandler 接口
func (a *AskServiceAdapter) Ask(ctx context.Context, question string) (string, []command.Reference, error) {
	req := AskRequest{Question: question}
	result, err := a.svc.Ask(ctx, req)
	if err != nil {
		return "", nil, err
	}

	refs := make([]command.Reference, 0, len(result.References))
	for _, r := range result.References {
		refs = append(refs, command.Reference{
			Title:     r.Title,
			FilePath:  r.FilePath,
			SourceURL: r.SourceURL,
			Snippet:   r.Snippet,
		})
	}

	return result.Answer, refs, nil
}
