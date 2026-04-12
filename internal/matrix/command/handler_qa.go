package command

import (
	"context"
	"fmt"
	"strings"
)

// QAHandler 知识库问答命令处理器
type QAHandler struct {
	BaseHandler
	askHandler AskHandler
}

// NewQAHandler 创建问答处理器
func NewQAHandler(askHandler AskHandler) *QAHandler {
	return &QAHandler{
		BaseHandler: BaseHandler{
			name:        "qa",
			description: "知识库问答",
			usage:       "<问题>",
		},
		askHandler: askHandler,
	}
}

// Handle 处理问答命令
func (h *QAHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	question := cmdCtx.Command.Args
	if question == "" {
		return &Response{
			Success: true,
			Message: "请输入问题，例如: `!问 什么是 RAG？`",
			IsHTML:  false,
		}, nil
	}

	answer, refs, err := h.askHandler.Ask(ctx, question)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 问答服务调用失败: %v", err),
		}, nil
	}

	msg := h.formatResponse(question, answer, refs)
	return &Response{
		Success: true,
		Message: msg,
		IsHTML:  false,
	}, nil
}

// formatResponse 格式化响应
func (h *QAHandler) formatResponse(question, answer string, refs []Reference) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("**Q:** %s\n\n", question))
	sb.WriteString(fmt.Sprintf("**A:** %s\n\n", answer))

	if len(refs) > 0 {
		sb.WriteString("📎 来源:\n")
		seen := make(map[string]bool)
		for _, ref := range refs {
			if seen[ref.FilePath] {
				continue
			}
			seen[ref.FilePath] = true
			sb.WriteString(fmt.Sprintf("• %s — %s\n", ref.Title, ref.FilePath))
		}
	}

	return sb.String()
}
