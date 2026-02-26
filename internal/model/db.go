package model

import (
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB initializes database connection
func InitDB(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// AutoMigrate runs database migrations
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Tag{},
		&DataSource{},
		&RSSFeed{},
		&WebhookConfig{},
		&WebhookHistory{},
		&DatasetMapping{},
		&ArticleTag{},
		&Setting{},
	); err != nil {
		return err
	}

	return SeedSettings(db)
}

// SeedSettings creates default settings if they don't exist
func SeedSettings(db *gorm.DB) error {
	defaults := []Setting{
		// API 配置
		{Key: "ragflow_base_url", Value: "", ValueType: "string", Category: "api", Description: "RagFlow API base URL"},
		{Key: "ragflow_api_key", Value: "", ValueType: "string", Category: "api", Description: "RagFlow API key", IsSecret: true},
		{Key: "n8n_webhook_base_url", Value: "", ValueType: "string", Category: "api", Description: "n8n Webhook base URL"},
		{Key: "n8n_api_base_url", Value: "", ValueType: "string", Category: "api", Description: "n8n API base URL"},
		{Key: "n8n_api_key", Value: "", ValueType: "string", Category: "api", Description: "n8n API key", IsSecret: true},
		// 功能开关
		{Key: "feature_auto_parse", Value: "true", ValueType: "bool", Category: "feature", Description: "自动解析上传的文档"},
		{Key: "feature_url_dedup", Value: "true", ValueType: "bool", Category: "feature", Description: "URL 去重检查"},
		{Key: "feature_ai_summary", Value: "false", ValueType: "bool", Category: "feature", Description: "AI 自动摘要"},
		// UI 配置
		{Key: "ui_page_size", Value: "20", ValueType: "int", Category: "ui", Description: "默认分页大小"},
		{Key: "ui_theme", Value: "system", ValueType: "string", Category: "ui", Description: "界面主题 (light/dark/system)"},
	}

	for _, s := range defaults {
		var count int64
		db.Model(&Setting{}).Where("key = ?", s.Key).Count(&count)
		if count == 0 {
			if err := db.Create(&s).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
