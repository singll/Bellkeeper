package command

import (
	"context"
	"fmt"
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
	healthChecker interface {
		Check(ctx context.Context) (map[string]interface{}, error)
	}
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

// NewStatusHandlerWithChecker creates a status handler with a health checker
func NewStatusHandlerWithChecker(checker interface {
	Check(ctx context.Context) (map[string]interface{}, error)
}) *StatusHandler {
	return &StatusHandler{
		BaseHandler: BaseHandler{
			name:        "status",
			description: "显示系统状态",
			usage:       "",
		},
		healthChecker: checker,
	}
}

func (h *StatusHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	if h.healthChecker == nil {
		return &Response{
			Success: true,
			Message: "⚠️ 健康检查服务未配置",
			IsHTML:  false,
		}, nil
	}

	result, err := h.healthChecker.Check(ctx)
	if err != nil {
		return &Response{
			Success: false,
			Message: "❌ 健康检查失败: " + err.Error(),
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("**Bellkeeper 健康状态**\n\n")

	if status, ok := result["status"].(string); ok {
		icon := "✅"
		if status != "healthy" {
			icon = "⚠️"
		}
		sb.WriteString(icon + " 整体状态: " + status + "\n\n")
	}

	if services, ok := result["services"].(map[string]interface{}); ok {
		sb.WriteString("**服务状态:**\n")
		for name, svc := range services {
			sb.WriteString("- " + name + ": " + fmt.Sprint(svc) + "\n")
		}
	}

	if metrics, ok := result["metrics"].(map[string]interface{}); ok {
		sb.WriteString("\n**指标:**\n")
		for k, v := range metrics {
			sb.WriteString("- " + k + ": " + fmt.Sprint(v) + "\n")
		}
	}

	return &Response{
		Success: true,
		Message: sb.String(),
		IsHTML:  true,
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

// ShortcutAliasHandler wraps a handler and injects a subcommand into Argv/Args
// before delegating. Used for commands like "!列表" → DirectMemosHandler with Argv[0]="列表".
type ShortcutAliasHandler struct {
	BaseHandler
	subCommand string
	wrapped    Handler
}

func NewShortcutAliasHandler(name, subCommand string, wrapped Handler) *ShortcutAliasHandler {
	return &ShortcutAliasHandler{
		BaseHandler: BaseHandler{
			name:        name,
			description: wrapped.Description(),
			usage:       wrapped.Usage(),
		},
		subCommand: subCommand,
		wrapped:    wrapped,
	}
}

func (h *ShortcutAliasHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	// Clone context and command to avoid mutating the original
	modified := *cmdCtx
	modCmd := *cmdCtx.Command
	// Prepend subcommand to Argv
	modCmd.Argv = append([]string{h.subCommand}, cmdCtx.Command.Argv...)
	// Rebuild Args with subcommand prefix
	if cmdCtx.Command.Args != "" {
		modCmd.Args = h.subCommand + " " + cmdCtx.Command.Args
	} else {
		modCmd.Args = h.subCommand
	}
	modified.Command = &modCmd
	return h.wrapped.Handle(ctx, &modified)
}

// HealthHandler handles !health command - shows detailed system status
type HealthHandler struct {
	BaseHandler
	adminSvc interface {
		GetStats(ctx context.Context) (map[string]interface{}, error)
	}
}

func NewHealthHandler(adminSvc interface {
	GetStats(ctx context.Context) (map[string]interface{}, error)
}) *HealthHandler {
	return &HealthHandler{
		BaseHandler: BaseHandler{
			name:        "health",
			description: "显示系统健康状态",
			usage:       "",
		},
		adminSvc: adminSvc,
	}
}

func (h *HealthHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	stats, err := h.adminSvc.GetStats(ctx)
	if err != nil {
		return &Response{
			Success: false,
			Message: "❌ 无法获取系统状态: " + err.Error(),
		}, nil
	}

	msg := "**Bellkeeper 健康状态**\n\n" +
		"✅ 服务运行正常\n\n" +
		"**统计信息:**\n" +
		"- 房间数: " + fmt.Sprint(stats["rooms"]) + "\n" +
		"- 活跃房间: " + fmt.Sprint(stats["active_rooms"]) + "\n" +
		"- 命令数: " + fmt.Sprint(stats["commands"]) + "\n" +
		"- 24h 事件数: " + fmt.Sprint(stats["events_24h"]) + "\n" +
		"- 24h 通知数: " + fmt.Sprint(stats["notifications_24h"])

	return &Response{
		Success: true,
		Message: msg,
		IsHTML:  true,
	}, nil
}

// RoomsHandler handles !rooms command - lists Matrix rooms
type RoomsHandler struct {
	BaseHandler
	adminSvc interface {
		ListRooms(ctx context.Context) ([]*RoomResponse, error)
	}
}

// RoomResponse is the room info type
type RoomResponse struct {
	RoomID   string `json:"room_id"`
	Name     string `json:"room_name"`
	Type     string `json:"room_type"`
	IsActive bool   `json:"is_active"`
}

func NewRoomsHandler(adminSvc interface {
	ListRooms(ctx context.Context) ([]*RoomResponse, error)
}) *RoomsHandler {
	return &RoomsHandler{
		BaseHandler: BaseHandler{
			name:        "rooms",
			description: "列出 Matrix 房间",
			usage:       "",
		},
		adminSvc: adminSvc,
	}
}

func (h *RoomsHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	rooms, err := h.adminSvc.ListRooms(ctx)
	if err != nil {
		return &Response{
			Success: false,
			Message: "❌ 无法获取房间列表: " + err.Error(),
		}, nil
	}

	if len(rooms) == 0 {
		return &Response{
			Success: true,
			Message: "**Matrix 房间列表**\n\n暂无注册的房间",
			IsHTML:  true,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("**Matrix 房间列表**\n\n")
	for _, r := range rooms {
		status := "🟢"
		if !r.IsActive {
			status = "🔴"
		}
		name := r.Name
		if name == "" {
			name = "(未命名)"
		}
		sb.WriteString(status + " " + name + "\n")
		sb.WriteString("   类型: " + r.Type + "\n")
	}

	return &Response{
		Success: true,
		Message: sb.String(),
		IsHTML:  true,
	}, nil
}

// CommandsHandler handles !commands command - lists registered commands
type CommandsHandler struct {
	BaseHandler
	listCommands func() []string
}

func NewCommandsHandler(listCommands func() []string) *CommandsHandler {
	return &CommandsHandler{
		BaseHandler: BaseHandler{
			name:        "commands",
			description: "列出所有可用命令",
			usage:       "",
		},
		listCommands: listCommands,
	}
}

func (h *CommandsHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	cmds := h.listCommands()
	if len(cmds) == 0 {
		return &Response{
			Success: true,
			Message: "**可用命令**\n\n暂无注册的命令",
			IsHTML:  true,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("**可用命令 (" + fmt.Sprint(len(cmds)) + ")**\n\n")
	for _, cmd := range cmds {
		sb.WriteString("• " + cmd + "\n")
	}
	sb.WriteString("\n发送 `!help <命令>` 查看详细帮助")

	return &Response{
		Success: true,
		Message: sb.String(),
		IsHTML:  true,
	}, nil
}
