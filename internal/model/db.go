package model

import (
	"log"
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
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Minute)

	return db, nil
}

// AutoMigrate runs database migrations
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Tag{},
		&RSSFeed{},
		&DatasetMapping{},
		&ArticleTag{},
		&Setting{},
		&LLMProxyLog{},
	); err != nil {
		return err
	}

	if err := SeedSettings(db); err != nil {
		return err
	}

	return SeedDatasetMappings(db)
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

// SeedDatasetMappings creates default dataset mappings and tag associations if they don't exist.
// DatasetID is left empty as a placeholder — must be filled with real RAGFlow dataset UUIDs
// via the Web UI or API after deployment.
func SeedDatasetMappings(db *gorm.DB) error {
	type mappingSeed struct {
		Name        string
		DisplayName string
		IsDefault   bool
		TagName     string
		TagColor    string
	}

	seeds := []mappingSeed{
		{Name: "security-tech", DisplayName: "网络安全知识库", IsDefault: false, TagName: "security", TagColor: "#F56C6C"},
		{Name: "ai-tech", DisplayName: "人工智能知识库", IsDefault: false, TagName: "ai", TagColor: "#409EFF"},
		{Name: "dev-tech", DisplayName: "编程开发知识库", IsDefault: false, TagName: "programming", TagColor: "#67C23A"},
		{Name: "daily-digest", DisplayName: "综合资讯", IsDefault: true, TagName: "general", TagColor: "#909399"},
	}

	for _, s := range seeds {
		// Ensure tag exists
		var tag Tag
		result := db.Where("name = ?", s.TagName).First(&tag)
		if result.Error != nil {
			tag = Tag{Name: s.TagName, Color: s.TagColor}
			if err := db.Create(&tag).Error; err != nil {
				log.Printf("warn: failed to seed tag %q: %v", s.TagName, err)
				continue
			}
		}

		// Ensure dataset mapping exists (don't overwrite existing records)
		var mapping DatasetMapping
		result = db.Where("name = ?", s.Name).First(&mapping)
		if result.Error != nil {
			mapping = DatasetMapping{
				Name:        s.Name,
				DisplayName: s.DisplayName,
				DatasetID:   "", // placeholder — fill via UI/API
				IsDefault:   s.IsDefault,
				IsActive:    true,
				ParserID:    "naive",
			}
			if err := db.Create(&mapping).Error; err != nil {
				log.Printf("warn: failed to seed dataset mapping %q: %v", s.Name, err)
				continue
			}
			log.Printf("info: seeded dataset mapping %q (display: %s)", s.Name, s.DisplayName)
		}

		// Ensure tag-dataset association exists
		var count int64
		db.Table("dataset_mapping_tags").
			Where("mapping_id = ? AND tag_id = ?", mapping.ID, tag.ID).
			Count(&count)
		if count == 0 {
			if err := db.Model(&mapping).Association("Tags").Append(&tag); err != nil {
				log.Printf("warn: failed to associate tag %q with dataset %q: %v", s.TagName, s.Name, err)
			}
		}
	}

	return nil
}
