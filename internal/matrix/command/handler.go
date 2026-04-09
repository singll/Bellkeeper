package command

import (
	"context"
	"strings"
)

// Context holds command execution context
type Context struct {
	RoomID   string // Room where command was issued
	Sender   string // User who issued the command
	EventID  string // Event ID of the command message
	Command  *ParsedCommand
	BotUserID string
}

// Response represents command execution result
type Response struct {
	Success bool
	Message string // Response message to send back
	IsHTML  bool   // If true, Message is HTML formatted
}

// Handler is the interface for command handlers
type Handler interface {
	// Handle executes the command
	Handle(ctx context.Context, cmdCtx *Context) (*Response, error)

	// Name returns the handler type name
	Name() string

	// Description returns human-readable description
	Description() string

	// Usage returns usage example
	Usage() string
}

// BaseHandler provides common functionality for handlers
type BaseHandler struct {
	name        string
	description string
	usage       string
}

func (h *BaseHandler) Name() string         { return h.name }
func (h *BaseHandler) Description() string { return h.description }
func (h *BaseHandler) Usage() string       { return h.usage }

// HelpHandler handles !help command
type HelpHandler struct {
	BaseHandler
	router *Router
}

// NewHelpHandler creates a help handler
func NewHelpHandler(router *Router) *HelpHandler {
	return &HelpHandler{
		BaseHandler: BaseHandler{
			name:        "help",
			description: "显示所有可用命令",
			usage:       "[命令名]",
		},
		router: router,
	}
}

func (h *HelpHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	// If specific command requested
	if len(cmdCtx.Command.Argv) > 0 {
		cmdName := cmdCtx.Command.Argv[0]
		handler, ok := h.router.GetHandler(cmdName)
		if !ok {
			return &Response{
				Success: false,
				Message: "未找到命令: " + cmdName,
			}, nil
		}
		return &Response{
			Success: true,
			Message: FormatHelp(handler.Name(), handler.Description(), handler.Usage()),
			IsHTML:  true,
		}, nil
	}

	// List all commands
	var sb strings.Builder
	sb.WriteString("**可用命令:**\n\n")

	for _, name := range h.router.ListCommands() {
		handler, _ := h.router.GetHandler(name)
		sb.WriteString(FormatHelp(name, handler.Description(), handler.Usage()))
	}

	sb.WriteString("\n发送 `!help <命令名>` 查看详细帮助。")

	return &Response{
		Success: true,
		Message: sb.String(),
		IsHTML:  true,
	}, nil
}

// StatusHandler handles !status command
type StatusHandler struct {
	BaseHandler
}

// NewStatusHandler creates a status handler
func NewStatusHandler() *StatusHandler {
	return &StatusHandler{
		BaseHandler: BaseHandler{
			name:        "status",
			description: "显示系统状态",
			usage:       "",
		},
	}
}

func (h *StatusHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	return &Response{
		Success: true,
		Message: "✅ Bellkeeper 运行正常\n\n" +
			"- Matrix Gateway: 在线\n" +
			"- Notification Gateway: 在线\n" +
			"- Command Router: 在线",
		IsHTML: false,
	}, nil
}

// PingHandler handles !ping command
type PingHandler struct {
	BaseHandler
}

func NewPingHandler() *PingHandler {
	return &PingHandler{
		BaseHandler: BaseHandler{
			name:        "ping",
			description: "测试机器人响应",
			usage:       "",
		},
	}
}

func (h *PingHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	return &Response{
		Success: true,
		Message: "🏓 Pong!",
		IsHTML:  false,
	}, nil
}

// AliasHandler wraps another handler with a different name
type AliasHandler struct {
	BaseHandler
	wrapped Handler
}

func NewAliasHandler(name string, wrapped Handler) *AliasHandler {
	return &AliasHandler{
		BaseHandler: BaseHandler{
			name:        name,
			description: wrapped.Description(),
			usage:       wrapped.Usage(),
		},
		wrapped: wrapped,
	}
}

func (h *AliasHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	return h.wrapped.Handle(ctx, cmdCtx)
}
