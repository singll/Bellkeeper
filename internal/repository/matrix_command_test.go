package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestMatrixCommandRepository_CreateAndGetByName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixCommandRepository(db)

	cmd := &model.MatrixCommand{
		CommandName:     "help",
		HandlerType:     "builtin_help",
		PermissionLevel: "user",
		RoomScope:       "any",
		IsActive:        true,
		Description:     "Show help",
		UsageExample:    "!help",
	}
	assertNoError(t, repo.Create(cmd), "Create")
	assertEqual(t, cmd.ID > 0, true)

	got, err := repo.GetByName("help")
	assertNoError(t, err, "GetByName")
	assertEqual(t, got.CommandName, "help")
	assertEqual(t, got.HandlerType, "builtin_help")
}

func TestMatrixCommandRepository_CreateHandlerConfigNil(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixCommandRepository(db)

	cmd := &model.MatrixCommand{
		CommandName: "ping",
		HandlerType: "builtin_ping",
		IsActive:    true,
	}
	assertNoError(t, repo.Create(cmd), "Create nil config")

	got, err := repo.GetByName("ping")
	assertNoError(t, err, "GetByName")
	assertEqual(t, got.HandlerConfig != nil, true)
	assertEqual(t, *got.HandlerConfig, "{}")
}

func TestMatrixCommandRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixCommandRepository(db)

	assertNoError(t, repo.Create(&model.MatrixCommand{CommandName: "help", HandlerType: "builtin", IsActive: true, HandlerConfig: strPtr("{}")}), "Create 1")
	assertNoError(t, repo.Create(&model.MatrixCommand{CommandName: "status", HandlerType: "builtin", IsActive: false, HandlerConfig: strPtr("{}")}), "Create 2")

	commands, err := repo.List(false)
	assertNoError(t, err, "List all")
	assertEqual(t, len(commands), 2)

	commands, err = repo.List(true)
	assertNoError(t, err, "List active")
	for _, cmd := range commands {
		assertEqual(t, cmd.IsActive, true)
	}
}
