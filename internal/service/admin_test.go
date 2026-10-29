package service

import (
	"context"
	"testing"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

func TestUpdateCommandCanReenableDisabledCommand(t *testing.T) {
	db := setupEnqueueTestDB(t)
	if err := db.AutoMigrate(&model.MatrixCommand{}); err != nil {
		t.Fatal(err)
	}
	const name = "test-reenable"
	if err := db.Where("command_name = ?", name).Delete(&model.MatrixCommand{}).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewMatrixCommandRepository(db)
	if err := repo.Create(&model.MatrixCommand{CommandName: name, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	svc := NewAdminService(&repository.Repositories{MatrixCommand: repo}, nil)
	for _, active := range []bool{false, true} {
		if err := svc.UpdateCommand(context.Background(), name, map[string]interface{}{"is_active": active}); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetConfigByName(name)
		if err != nil || got.IsActive != active {
			t.Fatalf("active = %#v, %v; want %v", got, err, active)
		}
	}
	if err := svc.UpdateCommand(context.Background(), "missing-command", map[string]interface{}{"is_active": true}); err == nil {
		t.Fatal("missing command accepted")
	}
}

func TestDisabledMemosDoesNotRegisterWebhookFallback(t *testing.T) {
	db := setupEnqueueTestDB(t)
	if err := db.AutoMigrate(&model.MatrixCommand{}); err != nil {
		t.Fatal(err)
	}
	repos := &repository.Repositories{MatrixCommand: repository.NewMatrixCommandRepository(db)}
	for _, enabled := range []bool{false, true} {
		svc := NewCommandService(config.MatrixConfig{CommandPrefix: "!"},
			config.N8NConfig{WebhookBaseURL: "https://example.com/webhook"},
			config.MemosConfig{Enabled: enabled}, repos, nil, []string{"@owner:test"})
		_, registered := svc.GetRouter().GetHandler("memos")
		if registered != enabled {
			t.Fatalf("Memos registered = %v, enabled = %v", registered, enabled)
		}
	}
}
