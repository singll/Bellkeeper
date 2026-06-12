package service

import (
	"context"
	"log"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/matrix/command"
	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/repository"
)

// CommandService handles Matrix command execution
type CommandService struct {
	cfg    config.MatrixConfig
	n8nCfg config.N8NConfig
	memosCfg config.MemosConfig
	router *command.Router
	client *gateway.Client
	repos  *repository.Repositories
}

// NewCommandService creates a new command service
func NewCommandService(
	cfg config.MatrixConfig,
	n8nCfg config.N8NConfig,
	memosCfg config.MemosConfig,
	repos *repository.Repositories,
	client *gateway.Client,
	adminUsers []string,
) *CommandService {
	// Use provided adminUsers or fallback to default
	if len(adminUsers) == 0 {
		adminUsers = []string{"@singll:" + defaults.DefaultMatrixDomain}
	}

	svc := &CommandService{
		cfg:      cfg,
		n8nCfg:   n8nCfg,
		memosCfg: memosCfg,
		repos:    repos,
		client:   client,
	}

	// Create router with command prefix and admin users from config
	svc.router = command.NewRouter(cfg.CommandPrefix, repos, adminUsers)

	// Register command handlers
	svc.registerHandlers()

	return svc
}

// registerHandlers registers all command handlers
func (s *CommandService) registerHandlers() {
	// Register built-in handlers
	// (Already registered by NewRouter)

	// Register Memos handlers
	// Priority: DirectMemosHandler (direct API) > N8NTriggerHandler (webhook fallback)
	if s.memosCfg.Enabled && s.memosCfg.BaseURL != "" {
		// Direct Memos handler - talks to Memos API directly
		directMemos := command.NewDirectMemosHandler(s.memosCfg.BaseURL, s.memosCfg.APIToken)
		s.router.RegisterHandler(directMemos) // registers as "待办"
		// Register shortcut aliases pointing to DirectMemosHandler
		s.router.RegisterHandler(command.NewShortcutAliasHandler("列表", "列表", directMemos))
		s.router.RegisterHandler(command.NewShortcutAliasHandler("新增", "新增", directMemos))
		s.router.RegisterHandler(command.NewShortcutAliasHandler("完成", "完成", directMemos))
		log.Printf("[Command] registered direct Memos handler with aliases")
	} else if s.n8nCfg.WebhookBaseURL != "" {
		// Fallback: n8n webhook
		memosWebhook := s.n8nCfg.WebhookBaseURL + "/memos-todo"
		memosHandler := command.NewN8NTriggerHandler("memos", memosWebhook)
		s.router.RegisterHandler(memosHandler)
		s.router.RegisterHandler(command.NewAliasHandler("列表", memosHandler))
		s.router.RegisterHandler(command.NewAliasHandler("新增", memosHandler))
		s.router.RegisterHandler(command.NewAliasHandler("完成", memosHandler))
		log.Printf("[Command] registered n8n Memos handler with aliases")
	}

	// Register QA handlers
	// Note: QA handlers are now registered via SetKnowledgeHandlers
	// This allows using the new AskService and SearchService instead of n8n webhooks

	log.Printf("[Command] registered %d commands", len(s.router.ListCommands()))
}

// GetRouter returns the command router
func (s *CommandService) GetRouter() *command.Router {
	return s.router
}

// SetKnowledgeHandlers sets the knowledge base command handlers
// This allows using the new AskService and SearchService instead of n8n webhooks
func (s *CommandService) SetKnowledgeHandlers(askHandler command.AskHandler, searchHandler command.SearchHandler) {
	qaHandler := command.NewQAHandler(askHandler)
	s.router.RegisterHandler(qaHandler)
	s.router.RegisterHandler(command.NewAliasHandler("问", qaHandler))

	searchMatrixHandler := command.NewMatrixSearchHandler(searchHandler)
	s.router.RegisterHandler(searchMatrixHandler)
	s.router.RegisterHandler(command.NewAliasHandler("搜", searchMatrixHandler))
	s.router.RegisterHandler(command.NewAliasHandler("search", searchMatrixHandler))

	log.Printf("[Command] registered knowledge handlers (ask and search)")
}

// ExecuteMessage processes a Matrix message and executes if it's a command
func (s *CommandService) ExecuteMessage(ctx context.Context, roomID, sender, eventID, content string) error {
	// Parse and route command
	response, isCommand, err := s.router.ExecuteFromMessage(ctx, roomID, sender, eventID, content)
	if err != nil {
		return err
	}

	if !isCommand {
		return nil // Not a command
	}

	// Send response back to room
	if response == nil {
		return nil
	}

	if response.IsHTML {
		_, err = s.client.SendHTMLMessage(ctx, roomID, response.Message, stripHTML(response.Message))
	} else {
		_, err = s.client.SendMessage(ctx, roomID, response.Message)
	}

	if err != nil {
		log.Printf("[Command] failed to send response: %v", err)
		return err
	}

	return nil
}

// ListCommands returns all available commands
func (s *CommandService) ListCommands() []string {
	return s.router.ListCommands()
}

// GetHelpText returns the help text for all commands
func (s *CommandService) GetHelpText() string {
	return s.router.GetHelpText()
}

// SetAdminService sets the admin service and registers admin commands
func (s *CommandService) SetAdminService(adminSvc *AdminService) {
	// Register health command
	s.router.RegisterHandler(command.NewHealthHandler(adminSvc))

	// Register rooms command - need to wrap adminSvc with type conversion
	type roomLister interface {
		ListRooms(ctx context.Context) ([]*command.RoomResponse, error)
	}
	s.router.RegisterHandler(command.NewRoomsHandler(roomListerAdapter{svc: adminSvc}))

	// Register commands list command
	s.router.RegisterHandler(command.NewCommandsHandler(s.ListCommands))

	log.Printf("[Command] registered admin commands")
}

// SetHealthChecker sets the health service and registers the status handler with real health check
func (s *CommandService) SetHealthChecker(hs *HealthService) {
	s.router.RegisterHandler(command.NewStatusHandlerWithChecker(healthCheckerAdapter{svc: hs}))
	log.Printf("[Command] registered status handler with health checker")
}

// healthCheckerAdapter adapts HealthService.Detailed() to the healthChecker interface
type healthCheckerAdapter struct {
	svc *HealthService
}

func (a healthCheckerAdapter) Check(ctx context.Context) (map[string]interface{}, error) {
	detailed := a.svc.Detailed()
	result := map[string]interface{}{
		"status":  detailed.Status,
		"version": detailed.Version,
	}
	if len(detailed.Services) > 0 {
		result["services"] = detailed.Services
	}
	if len(detailed.Metrics) > 0 {
		result["metrics"] = detailed.Metrics
	}
	return result, nil
}

// roomListerAdapter adapts AdminService.ListRooms to use command.RoomResponse
type roomListerAdapter struct {
	svc interface {
		ListRooms(ctx context.Context) ([]*RoomResponse, error)
	}
}

func (a roomListerAdapter) ListRooms(ctx context.Context) ([]*command.RoomResponse, error) {
	rooms, err := a.svc.ListRooms(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*command.RoomResponse, len(rooms))
	for i, r := range rooms {
		result[i] = &command.RoomResponse{
			RoomID:   r.RoomID,
			Name:     r.Name,
			Type:     r.Type,
			IsActive: r.IsActive,
		}
	}
	return result, nil
}
