package command

import (
	"context"
	"fmt"
	"strings"
)

// ModelManager 抽象 agent 的「按用户模型组覆盖」能力。
// command 包不 import service/agent，靠此窄接口解耦（仿 ResetHandler 成例）。
type ModelManager interface {
	SetUserModel(ctx context.Context, userID, group string) error
	CurrentUserModel(ctx context.Context, userID string) (string, error)
}

// ModelGroupInfo 是 !model 列表展示用的模型组信息。
type ModelGroupInfo struct {
	Name        string
	Description string
}

// ModelHandler 处理 !model：查看/切换「发言用户自己」的对话模型组（按用户持久、跨房间生效）。
type ModelHandler struct {
	BaseHandler
	list func() ([]ModelGroupInfo, error)
	mgr  ModelManager
}

// NewModelHandler 创建 !model 处理器。
func NewModelHandler(list func() ([]ModelGroupInfo, error), mgr ModelManager) *ModelHandler {
	return &ModelHandler{
		BaseHandler: BaseHandler{
			name:        "model",
			description: "查看/切换你的对话模型组",
			usage:       "[模型组名 | 默认]",
		},
		list: list,
		mgr:  mgr,
	}
}

// Handle 处理 !model 命令。
func (h *ModelHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	// 无参：列出可用模型组 + 当前生效
	if len(cmdCtx.Command.Argv) == 0 {
		return h.listModels(ctx, cmdCtx.Sender)
	}

	arg := cmdCtx.Command.Argv[0]

	// 恢复默认（清除自己的覆盖）
	if arg == "默认" || arg == "default" || arg == "reset" {
		if err := h.mgr.SetUserModel(ctx, cmdCtx.Sender, ""); err != nil {
			return &Response{Success: false, Message: "❌ 重置失败: " + err.Error()}, nil
		}
		return &Response{Success: true, Message: "✅ 已恢复为房间默认模型组"}, nil
	}

	// 切换：校验是否为合法模型组
	groups, err := h.list()
	if err != nil {
		return &Response{Success: false, Message: "❌ 无法获取模型组列表: " + err.Error()}, nil
	}
	valid := false
	for _, g := range groups {
		if g.Name == arg {
			valid = true
			break
		}
	}
	if !valid {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("未知模型组: %s\n发送 `!model` 查看可用模型组。", arg),
		}, nil
	}

	if err := h.mgr.SetUserModel(ctx, cmdCtx.Sender, arg); err != nil {
		return &Response{Success: false, Message: "❌ 切换失败: " + err.Error()}, nil
	}
	return &Response{
		Success: true,
		Message: fmt.Sprintf("✅ 已将你的对话模型组切换为 %s（仅影响你自己，跨房间生效；!reset 不会清除）", arg),
	}, nil
}

// listModels 列出可用模型组并标注当前用户生效项。
func (h *ModelHandler) listModels(ctx context.Context, sender string) (*Response, error) {
	groups, err := h.list()
	if err != nil {
		return &Response{Success: false, Message: "❌ 无法获取模型组列表: " + err.Error()}, nil
	}

	current, err := h.mgr.CurrentUserModel(ctx, sender)
	if err != nil {
		current = ""
	}

	var sb strings.Builder
	sb.WriteString("可用对话模型组：\n")
	if len(groups) == 0 {
		sb.WriteString("（暂无配置的模型组）\n")
	} else {
		for _, g := range groups {
			marker := "•"
			if g.Name == current {
				marker = "✅"
			}
			if g.Description != "" {
				sb.WriteString(fmt.Sprintf("%s %s — %s\n", marker, g.Name, g.Description))
			} else {
				sb.WriteString(fmt.Sprintf("%s %s\n", marker, g.Name))
			}
		}
	}
	sb.WriteString(fmt.Sprintf("\n你当前生效: %s\n", current))
	sb.WriteString("切换: !model <模型组名>；恢复默认: !model 默认")

	return &Response{
		Success: true,
		Message: sb.String(),
	}, nil
}
