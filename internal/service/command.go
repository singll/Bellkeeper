package service

import (
	"context"
	"log"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/matrix/command"
	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/repository"
)

// CommandService handles Matrix command execution
type CommandService struct {
	cfg    config.MatrixConfig
	n8nCfg config.N8NConfig
	router *command.Router
	client *gateway.Client
	repos  *repository.Repositories
}

// NewCommandService creates a new command service
func NewCommandService(
	cfg config.MatrixConfig,
	n8nCfg config.N8NConfig,
	repos *repository.Repositories,
	client *gateway.Client,
) *CommandService {
	svc := &CommandService{
		cfg:    cfg,
		n8nCfg: n8nCfg,
		repos:  repos,
		client: client,
	}

	// Create router with command prefix and admin users
	svc.router = command.NewRouter(cfg.CommandPrefix, repos, []string{"@singll:matrix.singll.net"})

	// Register command handlers
	svc.registerHandlers()

	return svc
}

// registerHandlers registers all command handlers
func (s *CommandService) registerHandlers() {
	// Register built-in handlers
	// (Already registered by NewRouter)

	// Register n8n webhook handlers
	if s.n8nCfg.WebhookBaseURL != "" {
		// Memos Todo webhook
		memosWebhook := s.n8nCfg.WebhookBaseURL + "/memos-todo"
		memosHandler := command.NewN8NTriggerHandler("memos", memosWebhook)
		s.router.RegisterHandler(memosHandler)
		s.router.RegisterHandler(command.NewAliasHandler("列表", memosHandler))
		s.router.RegisterHandler(command.NewAliasHandler("新增", memosHandler))
		s.router.RegisterHandler(command.NewAliasHandler("完成", memosHandler))

		// QA webhook
		qaWebhook := s.n8nCfg.WebhookBaseURL + "/qa"
		qaHandler := command.NewQAHandler(qaWebhook)
		s.router.RegisterHandler(qaHandler)
		s.router.RegisterHandler(command.NewAliasHandler("问", qaHandler))
		s.router.RegisterHandler(command.NewAliasHandler("搜", qaHandler))
		s.router.RegisterHandler(command.NewAliasHandler("search", qaHandler))

		// TODO: Add more n8n webhook handlers as needed
	}

	// Register direct handlers (when n8n not available)
	// These provide basic functionality without n8n
	// TODO: Implement direct Memos API handler

	log.Printf("[Command] registered %d commands", len(s.router.ListCommands()))
}

// GetRouter returns the command router
func (s *CommandService) GetRouter() *command.Router {
	return s.router
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
