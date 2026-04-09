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
	router *command.Router
	client *gateway.Client
	repos  *repository.Repositories
}

// NewCommandService creates a new command service
func NewCommandService(
	cfg config.MatrixConfig,
	repos *repository.Repositories,
	client *gateway.Client,
) *CommandService {
	svc := &CommandService{
		cfg:    cfg,
		repos:  repos,
		client: client,
	}

	// Create router with command prefix
	svc.router = command.NewRouter(cfg.CommandPrefix, repos)
	svc.router.SetAdminUsers([]string{"@singll:matrix.singll.net"}) // TODO: from config

	return svc
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
