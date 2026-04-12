package command

import "context"

// AskHandler 定义问答功能的接口
// 用于解耦 command 包和 service 包的循环依赖
type AskHandler interface {
	Ask(ctx context.Context, question string) (string, []Reference, error)
}

// Reference 引用
type Reference struct {
	Title     string
	FilePath  string
	SourceURL string
	Snippet   string
}
