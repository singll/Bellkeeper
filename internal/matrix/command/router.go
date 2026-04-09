package command

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/singll/bellkeeper/internal/repository"
)

// Router routes commands to handlers
type Router struct {
	handlers    map[string]Handler
	parser      *Parser
	repos       *repository.Repositories
	adminUsers  map[string]bool // set of admin user IDs
}

// NewRouter creates a new command router
func NewRouter(prefixes string, repos *repository.Repositories) *Router {
	r := &Router{
		handlers:   make(map[string]Handler),
		parser:     NewParser(prefixes),
		repos:      repos,
		adminUsers: make(map[string]bool),
	}

	// Register built-in handlers
	r.RegisterHandler(NewPingHandler())
	r.RegisterHandler(NewStatusHandler())
	r.RegisterHandler(NewHelpHandler(r)) // Help needs router reference

	// TODO: Load commands from database
	r.loadCommandsFromDB()

	return r
}

// RegisterHandler registers a command handler
func (r *Router) RegisterHandler(handler Handler) {
	r.handlers[handler.Name()] = handler
	log.Printf("[Command] registered handler: %s", handler.Name())

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
func (r *Router) Route(ctx context.Context, cmdCtx *Context) (*Response, error) {
	// Find handler
	handler, ok := r.GetHandler(cmdCtx.Command.Name)
	if !ok {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("未知命令: %s\n发送 `!help` 查看可用命令。", cmdCtx.Command.Name),
		}, nil
	}

	// Check permission
	if !r.checkPermission(cmdCtx, handler) {
		return &Response{
			Success: false,
			Message: "⚠️ 权限不足",
		}, nil
	}

	// Execute handler
	log.Printf("[Command] executing %s in room %s by %s", cmdCtx.Command.Name, cmdCtx.RoomID, cmdCtx.Sender)
	return handler.Handle(ctx, cmdCtx)
}

// checkPermission checks if user has permission to execute command
func (r *Router) checkPermission(cmdCtx *Context, handler Handler) bool {
	// Get handler config from DB (if any)
	cmd, err := r.repos.MatrixCommand.GetByName(cmdCtx.Command.Name)
	if err == nil && cmd != nil {
		// Check room scope
		switch cmd.RoomScope {
		case "admin_only":
			if !r.isAdmin(cmdCtx.Sender) {
				return false
			}
		}

		// Check permission level (simplified - just check admin)
		// TODO: Implement proper permission checking
	}

	return true
}

// isAdmin checks if user is an admin
func (r *Router) isAdmin(userID string) bool {
	return r.adminUsers[userID]
}

// SetAdminUsers sets the list of admin users
func (r *Router) SetAdminUsers(users []string) {
	r.adminUsers = make(map[string]bool)
	for _, u := range users {
		r.adminUsers[u] = true
	}
}

// loadCommandsFromDB loads commands from database
func (r *Router) loadCommandsFromDB() {
	// Load commands from DB and register custom handlers
	// This would typically create handler instances based on HandlerType
	// For now, we rely on built-in handlers
	log.Printf("[Command] loaded %d commands from DB", len(r.handlers))
}

// ExecuteFromMessage processes a raw Matrix message and executes if it's a command
func (r *Router) ExecuteFromMessage(ctx context.Context, roomID, sender, eventID, content string) (*Response, bool, error) {
	// Parse message
	parsed, ok := r.Parse(content)
	if !ok {
		return nil, false, nil // Not a command
	}

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
		return nil, true, err
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
