package command

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDisabledCommandCannotExecuteOrAppearInHelp(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=localhost port=15432 user=bellkeeper password=testpass dbname=bellkeeper_test sslmode=disable"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	schema := fmt.Sprintf("command_test_%d", time.Now().UnixNano())
	if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.Exec("SET search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.MatrixCommand{}, &model.MatrixRoom{}, &model.MatrixUserRole{}); err != nil {
		t.Fatal(err)
	}
	repos := &repository.Repositories{
		MatrixCommand:  repository.NewMatrixCommandRepository(db),
		MatrixRoom:     repository.NewMatrixRoomRepository(db),
		MatrixUserRole: repository.NewMatrixUserRoleRepository(db),
	}
	r := NewRouter("!", repos, []string{"@owner:test"})
	ctx := &Context{Sender: "@owner:test", RoomID: "!room:test", Command: &ParsedCommand{Name: "ping"}}
	result, err := r.Route(context.Background(), ctx)
	if err != nil || !result.Success {
		t.Fatalf("built-in ping failed: %#v %v", result, err)
	}
	cmd := &model.MatrixCommand{CommandName: "ping", IsActive: true, PermissionLevel: "guest"}
	if err := db.Create(cmd).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(cmd).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	result, err = r.Route(context.Background(), ctx)
	if err != nil || result.Success {
		t.Fatalf("disabled ping executed: %#v %v", result, err)
	}
	if strings.Contains(r.GetHelpText(), "!ping") {
		t.Fatal("disabled command advertised")
	}
	if err := db.Model(cmd).Update("is_active", true).Error; err != nil {
		t.Fatal(err)
	}
	result, err = r.Route(context.Background(), ctx)
	if err != nil || !result.Success {
		t.Fatalf("re-enabled ping failed: %#v %v", result, err)
	}
	if err := db.Migrator().DropTable(&model.MatrixCommand{}); err != nil {
		t.Fatal(err)
	}
	result, err = r.Route(context.Background(), ctx)
	if err != nil || result.Success {
		t.Fatalf("database error allowed execution: %#v %v", result, err)
	}
}
