package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/matrix/policy"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// Router routes commands to handlers
type Router struct {
	handlers    map[string]Handler
	parser     *Parser
	repos      *repository.Repositories
	policy     *policy.Checker
	n8nCfg     config.N8NConfig
	memosCfg   config.MemosConfig
}

// RouterConfig holds configuration for the router
type RouterConfig struct {
	N8NConfig   config.N8NConfig
	MemosConfig config.MemosConfig
}

// NewRouter creates a new command router
func NewRouter(prefixes string, repos *repository.Repositories, adminUsers []string) *Router {
	return NewRouterWithConfig(prefixes, repos, adminUsers, RouterConfig{})
}

// NewRouterWithConfig creates a new command router with config
func NewRouterWithConfig(prefixes string, repos *repository.Repositories, adminUsers []string, cfg RouterConfig) *Router {
	r := &Router{
		handlers:   make(map[string]Handler),
		parser:    NewParser(prefixes),
		repos:     repos,
		policy:    policy.NewChecker(repos, adminUsers),
		n8nCfg:   cfg.N8NConfig,
		memosCfg: cfg.MemosConfig,
	}

	// Register built-in handlers
	r.RegisterHandler(NewPingHandler())
	r.RegisterHandler(NewStatusHandler())
	r.RegisterHandler(NewHelpHandler(r)) // Help needs router reference

	// Load commands from database
	r.loadCommandsFromDB()

	return r
}

// SetPolicyChecker updates the policy checker (for hot reload)
func (r *Router) SetPolicyChecker(checker *policy.Checker) {
	r.policy = checker
}

// ReloadCommands clears dynamically loaded commands and reloads from the database.
// Built-in handlers (ping, status, help) are preserved.
func (r *Router) ReloadCommands() {
	builtins := map[string]bool{
		NewPingHandler().Name():   true,
		NewStatusHandler().Name(): true,
		NewHelpHandler(nil).Name(): true,
	}

	// Remove non-builtin handlers
	for name := range r.handlers {
		if !builtins[name] {
			delete(r.handlers, name)
		}
	}

	r.loadCommandsFromDB()
}

// RegisterHandler registers a command handler
func (r *Router) RegisterHandler(handler Handler) {
	r.handlers[handler.Name()] = handler
	middleware.GetLogger().Info("registered command handler", zap.String("name", handler.Name()))

	// Also register aliases if defined
	// (Future: can add alias support in Handler interface)
}

// GetHandler returns a handler by name
func (r *Router) GetHandler(name string) (Handler, bool) {
	h, ok := r.handlers[name]
	return h, ok
}

// ListCommands returns all registered command names
func (r *Router) ListCommands() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// Parse parses a message and returns a parsed command
func (r *Router) Parse(content string) (*ParsedCommand, bool) {
	return r.parser.Parse(content)
}

// Route routes a parsed command to the appropriate handler
// Note: Command logging is handled in ExecuteFromMessage for proper timing
func (r *Router) Route(ctx context.Context, cmdCtx *Context) (*Response, error) {
	// Find handler
	handler, ok := r.GetHandler(cmdCtx.Command.Name)
	if !ok {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("未知命令: %s\n发送 `!help` 查看可用命令。", cmdCtx.Command.Name),
		}, nil
	}

	// Check permission using policy engine
	if !r.checkPermission(ctx, cmdCtx) {
		return &Response{
			Success: false,
			Message: "⚠️ 权限不足",
		}, nil
	}

	// Execute handler
	middleware.GetLogger().Info("executing command",
		zap.String("command", cmdCtx.Command.Name),
		zap.String("room", cmdCtx.RoomID),
		zap.String("sender", cmdCtx.Sender))
	return handler.Handle(ctx, cmdCtx)
}

// checkPermission checks if user has permission to execute command using policy engine
func (r *Router) checkPermission(ctx context.Context, cmdCtx *Context) bool {
	// Get command config from DB
	cmd, err := r.repos.MatrixCommand.GetByName(cmdCtx.Command.Name)
	if err != nil || cmd == nil {
		// Command not in DB — allow if handler is registered (built-in/dynamic commands)
		// Built-in commands (ping, help, status, commands, health, rooms) may not have DB entries
		if _, exists := r.handlers[cmdCtx.Command.Name]; exists {
			middleware.GetLogger().Info("command not in DB, allowing registered handler", zap.String("command", cmdCtx.Command.Name))
			return true
		}
		middleware.GetLogger().Warn("command not found in DB or handlers", zap.String("command", cmdCtx.Command.Name))
		return false
	}

	// Check room scope first
	if cmd.RoomScope == "admin_only" && !r.policy.IsAdmin(cmdCtx.Sender) {
		middleware.GetLogger().Warn("command requires admin_only room scope", zap.String("command", cmdCtx.Command.Name))
		return false
	}

	// Use policy checker for permission level
	allowed, err := r.policy.CheckCommandPermission(ctx, cmdCtx.Sender, cmdCtx.RoomID, cmdCtx.Command.Name, cmd.PermissionLevel)
	if err != nil {
		middleware.GetLogger().Warn("permission check failed", zap.Error(err))
		return false
	}
	return allowed
}

// loadCommandsFromDB loads commands from database and registers handlers
func (r *Router) loadCommandsFromDB() {
	commands, err := r.repos.MatrixCommand.List(true) // activeOnly = true
	if err != nil {
		middleware.GetLogger().Error("failed to load commands from DB", zap.Error(err))
		return
	}

	registered := 0
	for _, cmd := range commands {
		// Skip if handler already registered (built-in handlers take priority)
		if _, exists := r.handlers[cmd.CommandName]; exists {
			continue
		}

		handler := r.createHandlerFromDB(&cmd)
		if handler != nil {
			r.RegisterHandler(handler)
			registered++
		}
	}

	middleware.GetLogger().Info("loaded commands from DB",
		zap.Int("new", registered), zap.Int("total", len(r.handlers)))
}

// createHandlerFromDB creates a handler instance from DB command config
func (r *Router) createHandlerFromDB(cmd *model.MatrixCommand) Handler {
	var config map[string]interface{}
	if cmd.HandlerConfig != nil && *cmd.HandlerConfig != "" {
		if err := json.Unmarshal([]byte(*cmd.HandlerConfig), &config); err != nil {
			log.Printf("[Router] failed to parse handler config for command %s: %v", cmd.CommandName, err)
		}
	}

	switch cmd.HandlerType {
	case "n8n_webhook":
		webhookURL := ""
		if url, ok := config["webhook_url"].(string); ok && url != "" {
			webhookURL = url
		} else if r.n8nCfg.WebhookBaseURL != "" {
			if path, ok := config["webhook_path"].(string); ok {
				webhookURL = r.n8nCfg.WebhookBaseURL + path
			}
		}
		if webhookURL == "" {
			middleware.GetLogger().Warn("skip command: no webhook URL configured", zap.String("command", cmd.CommandName))
			return nil
		}
		return NewN8NTriggerHandler(cmd.CommandName, webhookURL)

	case "knowledge_qa", "knowledge_search":
		return nil

	case "builtin_ping", "builtin_status", "builtin_help", "builtin_health", "builtin_rooms", "builtin_commands":
		return nil

	default:
		if webhookURL, ok := config["webhook_url"].(string); ok && webhookURL != "" {
			return NewN8NTriggerHandler(cmd.CommandName, webhookURL)
		}
		middleware.GetLogger().Warn("unknown handler type", zap.String("type", cmd.HandlerType), zap.String("command", cmd.CommandName))
		return nil
	}
}

// ExecuteFromMessage processes a raw Matrix message and executes if it's a command
// This is the main entry point that handles command logging
func (r *Router) ExecuteFromMessage(ctx context.Context, roomID, sender, eventID, content string) (*Response, bool, error) {
	// Parse message
	parsed, ok := r.Parse(content)
	if !ok {
		return nil, false, nil // Not a command
	}

	// Build args string for logging
	argsStr := parsed.Args

	// Determine handler type from registered handler
	handlerType := ""
	if handler, exists := r.GetHandler(parsed.Name); exists {
		handlerType = handler.Name()
	}

	// Create log entry BEFORE execution
	logEntry := &model.MatrixCommandLog{
		EventID:         eventID,
		RoomID:          roomID,
		Sender:          sender,
		CommandName:     parsed.Name,
		CommandArgs:     argsStr,
		HandlerType:     handlerType,
		ExecutionStatus: "pending",
		CreatedAt:       time.Now(),
	}

	// Insert log entry - use a new goroutine to avoid blocking command execution
	// But if repos is nil, skip logging
	if r.repos != nil {
		if err := r.repos.MatrixCommandLog.Create(logEntry); err != nil {
			middleware.GetLogger().Error("failed to create command log entry", zap.Error(err))
		}
	}

	startTime := time.Now()

	// Create context
	cmdCtx := &Context{
		RoomID:  roomID,
		Sender:  sender,
		EventID: eventID,
		Command: parsed,
	}

	// Route to handler
	response, err := r.Route(ctx, cmdCtx)
	if err != nil {
		// Log failure
		if r.repos != nil {
			durationMs := int(time.Since(startTime).Milliseconds())
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			}
			if completeErr := r.repos.MatrixCommandLog.Complete(eventID, "failed", errMsg, "", durationMs); completeErr != nil {
				middleware.GetLogger().Error("failed to update command log entry", zap.Error(completeErr))
			}
		}
		return nil, true, err
	}

	// Update log entry with result
	if r.repos != nil {
		durationMs := int(time.Since(startTime).Milliseconds())
		status := "success"
		errMsg := ""
		if !response.Success {
			status = "failed"
			errMsg = response.Message
		}
		if completeErr := r.repos.MatrixCommandLog.Complete(eventID, status, errMsg, "", durationMs); completeErr != nil {
			middleware.GetLogger().Error("failed to update command log entry", zap.Error(completeErr))
		}
	}

	return response, true, nil
}

// GetHelpText returns formatted help for all commands
func (r *Router) GetHelpText() string {
	var sb strings.Builder
	sb.WriteString("**可用命令:**\n\n")

	for _, name := range r.ListCommands() {
		handler, _ := r.GetHandler(name)
		sb.WriteString(FormatHelp(name, handler.Description(), handler.Usage()))
	}

	sb.WriteString("\n发送 `!help <命令名>` 查看详细帮助。")
	return sb.String()
}
