package command

import "context"

// PKBProposalActions 注入的知识骨架待批提议操作。实现放在 app 层（闭包包 pkb 包级函数），
// 以避开 command → pkb → service → command 的 import 环。
type PKBProposalActions struct {
	List    func() (string, error)
	Approve func(id string) (string, error)
	Reject  func(id string) (string, error)
}

// PKBProposalHandler 处理 !pkb——知识骨架「大动作」结构变更的过渡审批闸（ADR-0004 / Q12）：
// pkb-curate propose 把超影响半径阈值的变更落为待批提议并推到本房间，用户用本命令批准/驳回。
type PKBProposalHandler struct {
	BaseHandler
	actions PKBProposalActions
}

// NewPKBProposalHandler 构造 !pkb handler。
func NewPKBProposalHandler(actions PKBProposalActions) *PKBProposalHandler {
	return &PKBProposalHandler{
		BaseHandler: BaseHandler{
			name:        "pkb",
			description: "知识骨架待批提议：list 查看 / approve <id> 批准 / reject <id> 驳回",
			usage:       "list | approve <id> | reject <id>",
		},
		actions: actions,
	}
}

// Handle 分发 !pkb 子命令。
func (h *PKBProposalHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	argv := cmdCtx.Command.Argv
	sub := "list"
	if len(argv) > 0 {
		sub = argv[0]
	}
	var (
		msg string
		err error
	)
	switch sub {
	case "list":
		msg, err = h.actions.List()
	case "approve":
		if len(argv) < 2 {
			return &Response{Success: false, Message: "用法：!pkb approve <id>"}, nil
		}
		msg, err = h.actions.Approve(argv[1])
	case "reject":
		if len(argv) < 2 {
			return &Response{Success: false, Message: "用法：!pkb reject <id>"}, nil
		}
		msg, err = h.actions.Reject(argv[1])
	default:
		return &Response{Success: false, Message: "未知子命令。用法：!pkb list | approve <id> | reject <id>"}, nil
	}
	if err != nil {
		return &Response{Success: false, Message: "❌ " + err.Error()}, nil
	}
	return &Response{Success: true, Message: msg}, nil
}
