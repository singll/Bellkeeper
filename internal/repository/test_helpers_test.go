package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testDSN = "host=localhost port=15432 user=bellkeeper password=testpass dbname=bellkeeper_test sslmode=disable"

var sharedDB *gorm.DB

func init() {
	db, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic(fmt.Sprintf("open test postgres: %v", err))
	}
	db.Exec("CREATE SCHEMA IF NOT EXISTS repo_test")
	db.Exec("SET search_path TO repo_test, public")
	if err := db.AutoMigrate(allModels()...); err != nil {
		panic(fmt.Sprintf("auto migrate: %v", err))
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(10)
	sharedDB = db
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	truncateAll(t)
	seedUncategorizedTag(t)
	return sharedDB.Session(&gorm.Session{NewDB: true})
}

func seedUncategorizedTag(t *testing.T) {
	t.Helper()
	now := time.Now()
	if err := sharedDB.Exec(
		"INSERT INTO tags (id, name, description, color, created_at, updated_at, deleted_at) VALUES (0, 'uncategorized', '默认未分类', '#909399', ?, ?, null)",
		now, now,
	).Error; err != nil {
		t.Fatalf("seed uncategorized tag: %v", err)
	}
}

func truncateAll(t *testing.T) {
	t.Helper()
	tables := []string{
		"crawl_rule_trials", "crawl_extraction_rules", "crawl_jobs", "crawl_domain_profiles",
		"matrix_command_logs", "matrix_events", "matrix_notifications", "matrix_sync_state",
		"matrix_user_roles", "matrix_commands", "matrix_channels", "matrix_rooms",
		"llm_channel_balance_snapshots", "llm_channel_credentials", "llm_alert_events",
		"llm_proxy_logs", "llm_token_usage_daily", "llm_model_rate_limits",
		"llm_model_pricing", "llm_conversation_bindings", "llm_model_group_members",
		"llm_model_groups", "llm_jobs", "llm_tokens", "llm_channels",
		"log_alert_rules", "log_entries", "log_sources", "activity_logs",
		"dataset_mapping_tags", "article_tags", "dataset_mappings", "rss_tags", "rss_feeds", "tags",
		"settings",
	}
	for _, table := range tables {
		sharedDB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
	}
}

func allModels() []interface{} {
	return []interface{}{
		&model.Tag{},
		&model.RSSFeed{},
		&model.DatasetMapping{},
		&model.ArticleTag{},
		&model.Setting{},
		&model.LLMProxyLog{},
		&model.LLMChannel{},
		&model.LLMModelGroup{},
		&model.LLMModelGroupMember{},
		&model.LLMToken{},
		&model.LLMTokenUsageDaily{},
		&model.LLMModelPricing{},
		&model.LLMModelRateLimit{},
		&model.LLMConversationBinding{},
		&model.LLMAlertEvent{},
		&model.LLMChannelCredential{},
		&model.LLMChannelBalanceSnapshot{},
		&model.LLMJob{},
		&model.ActivityLog{},
		&model.LogSource{},
		&model.LogEntry{},
		&model.LogAlertRule{},
		&model.MatrixRoom{},
		&model.MatrixChannel{},
		&model.MatrixCommand{},
		&model.MatrixEvent{},
		&model.MatrixNotification{},
		&model.MatrixCommandLog{},
		&model.MatrixSyncState{},
		&model.MatrixUserRole{},
		&model.CrawlDomainProfile{},
		&model.CrawlExtractionRule{},
		&model.CrawlRuleTrial{},
		&model.CrawlJob{},
	}
}

func assertEqual(t *testing.T, got, want interface{}, msg ...string) {
	t.Helper()
	prefix := ""
	if len(msg) > 0 {
		prefix = msg[0] + ": "
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%sgot %v, want %v", prefix, got, want)
	}
}

func assertNoError(t *testing.T, err error, msg ...string) {
	t.Helper()
	if err != nil {
		prefix := ""
		if len(msg) > 0 {
			prefix = msg[0] + ": "
		}
		t.Fatalf("%s%v", prefix, err)
	}
}

func assertError(t *testing.T, err error, msg ...string) {
	t.Helper()
	if err == nil {
		prefix := ""
		if len(msg) > 0 {
			prefix = msg[0] + ": "
		}
		t.Fatalf("%sexpected error, got nil", prefix)
	}
}