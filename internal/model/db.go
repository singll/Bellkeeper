package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/pkg/crypto"
	"github.com/singll/bellkeeper/internal/pkg/envutil"
	"go.uber.org/zap"
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
		&LLMChannel{},
		&LLMModelGroup{},
		&LLMModelGroupMember{},
		&LLMToken{},
		&LLMTokenUsageDaily{},
		&LLMModelPricing{},
		&LLMModelRateLimit{},
		&LLMConversationBinding{},
		&LLMAlertEvent{},
		&LLMChannelCredential{},
		&LLMChannelBalanceSnapshot{},
		&ActivityLog{},
		&LogSource{},
		&LogEntry{},
		&LogAlertRule{},
		&MatrixRoom{},
		&MatrixChannel{},
		&MatrixCommand{},
		&MatrixEvent{},
		&MatrixNotification{},
		&MatrixCommandLog{},
		&MatrixSyncState{},
		&MatrixUserRole{},
			&CrawlSource{},
			&CrawlJob{},
	); err != nil {
		return err
	}

	if err := SeedSettings(db); err != nil {
		return err
	}

	if err := SeedUncategorizedTag(db); err != nil {
		return err
	}

	if err := SeedDatasetMappings(db); err != nil {
		return err
	}

	if err := SeedLogSources(db); err != nil {
		return err
	}

	if err := MigrateChannelCredentials(db); err != nil {
		middleware.GetLogger().Warn("credential migration failed (non-fatal)",
			zap.Error(err))
	}

	return SeedMatrixPlatform(db)
}

// AutoMigrateWithLLMSeed runs migrations and seeds LLM proxy config from YAML if DB is empty.
func AutoMigrateWithLLMSeed(db *gorm.DB, cfg *config.Config) error {
	if err := AutoMigrate(db); err != nil {
		return err
	}
	if err := SeedLLMProxyConfig(db, cfg.LLMProxy); err != nil {
		return err
	}
	return SeedDefaultLLMToken(db, cfg.Server.APIKey)
}

// SeedDefaultLLMToken seeds a single "default" token keyed off the server API key
// when the llm_tokens table is empty. This lets internal callers that authenticate
// with server.api_key resolve to a real token (non-zero token_id) so their usage is
// recorded in billing instead of bypassing it (LLMTokenAuth resolves the server key
// to this token). No-op when the table is non-empty or no server key is configured.
// (ROADMAP line 210.)
func SeedDefaultLLMToken(db *gorm.DB, serverAPIKey string) error {
	if serverAPIKey == "" {
		return nil
	}
	var count int64
	db.Model(&LLMToken{}).Count(&count)
	if count > 0 {
		return nil
	}
	prefix := serverAPIKey
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	token := LLMToken{
		Name:      "default",
		KeyHash:   HashKey(serverAPIKey),
		KeyPrefix: prefix,
		CallerID:  "default",
		Enabled:   true,
		// no quotas (0 = unlimited); empty allowed models/groups = all allowed
	}
	return db.Create(&token).Error
}

// SeedLLMProxyConfig imports channels and model groups from YAML config into DB
// on first startup (when tables are empty). Subsequent runs are no-ops.
func SeedLLMProxyConfig(db *gorm.DB, cfg config.LLMProxyConfig) error {
	var channelCount int64
	db.Model(&LLMChannel{}).Count(&channelCount)
	if channelCount > 0 {
		return nil // DB already has data, skip seeding
	}

	middleware.GetLogger().Info("seeding LLM proxy config from YAML (first-time migration)")

	// Seed channels
	for _, ch := range cfg.Channels {
		// Extract env var name from "${VAR_NAME}" pattern (pre-expansion)
		apiKeyEnv := ch.RawAPIKey
		if strings.HasPrefix(apiKeyEnv, "${") && strings.HasSuffix(apiKeyEnv, "}") {
			apiKeyEnv = apiKeyEnv[2 : len(apiKeyEnv)-1]
		}

		modelsJSON, err := json.Marshal(ch.Models)
		if err != nil {
			middleware.GetLogger().Warn("failed to marshal models for LLM channel",
				zap.String("channel", ch.Name), zap.Error(err))
			continue
		}
		channel := LLMChannel{
			Name:      ch.Name,
			BaseURL:   ch.BaseURL,
			APIKeyEnv: apiKeyEnv,
			RPM:       ch.RPM,
			RPD:       ch.RPD,
			Priority:  ch.Priority,
			IsFree:    ch.IsFree,
			IsEnabled: ch.IsEnabled,
			Models:    string(modelsJSON),
		}
		if err := db.Create(&channel).Error; err != nil {
			middleware.GetLogger().Warn("failed to seed LLM channel",
				zap.String("channel", ch.Name), zap.Error(err))
			continue
		}
		middleware.GetLogger().Info("seeded LLM channel",
			zap.String("channel", ch.Name), zap.Int("models", len(ch.Models)))
	}

	// Seed model groups
	for _, g := range cfg.ModelGroups {
		group := LLMModelGroup{
			Name:             g.Name,
			Description:      g.Description,
			Strategy:         g.Strategy,
			StickyTTLSeconds: g.StickyTTLSeconds,
		}
		for _, m := range g.Members {
			weight := m.Weight
			if weight <= 0 {
				weight = 1
			}
			group.Members = append(group.Members, LLMModelGroupMember{
				ChannelName: m.Channel,
				Model:       m.Model,
				Weight:      weight,
			})
		}
		if err := db.Create(&group).Error; err != nil {
			middleware.GetLogger().Warn("failed to seed LLM model group",
				zap.String("group", g.Name), zap.Error(err))
			continue
		}
		middleware.GetLogger().Info("seeded LLM model group",
			zap.String("group", g.Name), zap.Int("members", len(g.Members)))
	}

	return nil
}

// SeedSettings creates default settings if they don't exist
func SeedSettings(db *gorm.DB) error {
	defaults := []Setting{
		// API 配置
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

// SeedLogSources creates default log sources if they don't exist.
func SeedLogSources(db *gorm.DB) error {
	defaults := []LogSource{
		{Name: "bellkeeper-core", SourceType: "internal", Description: "Bellkeeper core modules"},
		{Name: "n8n-workflows", SourceType: "n8n", Description: "n8n automation workflows"},
		{Name: "matrix-bot", SourceType: "internal", Description: "Matrix bot commands"},
	}

	for _, s := range defaults {
		var count int64
		db.Model(&LogSource{}).Where("name = ?", s.Name).Count(&count)
		if count == 0 {
			if err := db.Create(&s).Error; err != nil {
				middleware.GetLogger().Warn("failed to seed log source",
					zap.String("name", s.Name), zap.Error(err))
				continue
			}
			middleware.GetLogger().Info("seeded log source",
				zap.String("name", s.Name), zap.String("type", s.SourceType))
		}
	}

	return nil
}

// SeedUncategorizedTag ensures a tag with id=0 exists for uncategorized articles.
// File ingestion creates ArticleTag records with TagID=0 by default.
func SeedUncategorizedTag(db *gorm.DB) error {
	var count int64
	db.Table("tags").Where("id = 0").Count(&count)
	if count == 0 {
		now := time.Now()
		if err := db.Exec(
			"INSERT INTO tags (id, name, description, color, created_at, updated_at) VALUES (0, 'uncategorized', '默认未分类', '#909399', ?, ?)",
			now, now,
		).Error; err != nil {
			return err
		}
		middleware.GetLogger().Info("seeded uncategorized tag (id=0)")
	}
	return nil
}

// SeedDatasetMappings creates default dataset mappings and tag associations if they don't exist.
// DatasetID is left empty as a placeholder — it will be auto-filled (e.g. UUID or slug) by
// the dataset service once a real index partition is bound via the Web UI or API.
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
				middleware.GetLogger().Warn("failed to seed tag",
					zap.String("tag", s.TagName), zap.Error(err))
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
				middleware.GetLogger().Warn("failed to seed dataset mapping",
					zap.String("name", s.Name), zap.Error(err))
				continue
			}
			middleware.GetLogger().Info("seeded dataset mapping",
				zap.String("name", s.Name), zap.String("display", s.DisplayName))
		}

		// Ensure tag-dataset association exists
		var count int64
		db.Table("dataset_mapping_tags").
			Where("dataset_mapping_id = ? AND tag_id = ?", mapping.ID, tag.ID).
			Count(&count)
		if count == 0 {
			if err := db.Model(&mapping).Association("Tags").Append(&tag); err != nil {
				middleware.GetLogger().Warn("failed to associate tag with dataset",
					zap.String("tag", s.TagName), zap.String("dataset", s.Name), zap.Error(err))
			}
		}
	}

	return nil
}

var balanceSecretKeys = []string{"access_secret", "session", "ak_secret", "secret_key"}

func MigrateChannelCredentials(db *gorm.DB) error {
	if !crypto.Enabled() {
		middleware.GetLogger().Warn("credential encryption is DISABLED (BELLKEEPER_CREDENTIAL_KEY unset); " +
			"migrated credentials will be stored as plaintext — set the key and restart before relying on encryption")
	}

	var channels []LLMChannel
	if err := db.Find(&channels).Error; err != nil {
		return fmt.Errorf("fetch channels for credential migration: %w", err)
	}

	for _, ch := range channels {
		var apiCredCount int64
		if err := db.Model(&LLMChannelCredential{}).Where("channel_id = ? AND purpose = ?", ch.ID, "api").Count(&apiCredCount).Error; err != nil {
			middleware.GetLogger().Warn("count api credentials failed, skipping channel",
				zap.String("channel", ch.Name), zap.Error(err))
			continue
		}
		if apiCredCount == 0 && ch.APIKeyEnv != "" {
			cred := LLMChannelCredential{
				ChannelID: ch.ID,
				Purpose:   "api",
				IsPreset:  true,
				Status:    "active",
			}
			now := time.Now()
			cred.LastRefreshedAt = &now
			if envutil.LooksLikeEnvVar(ch.APIKeyEnv) {
				cred.Source = "env"
				cred.EnvVarName = ch.APIKeyEnv
				cred.Label = ch.APIKeyEnv
				if err := db.Create(&cred).Error; err != nil {
					middleware.GetLogger().Warn("migrate api/env credential failed",
						zap.String("channel", ch.Name), zap.Error(err))
					continue
				}
			} else {
				cred.Source = "direct"
				cred.Label = "migrated-direct-key"
				enc, err := crypto.Encrypt(ch.APIKeyEnv)
				if err != nil {
					middleware.GetLogger().Warn("encrypt migrated direct key failed",
						zap.String("channel", ch.Name), zap.Error(err))
					continue
				}
				cred.CredentialJSON = enc
				if err := db.Create(&cred).Error; err != nil {
					middleware.GetLogger().Warn("migrate api/direct credential failed",
						zap.String("channel", ch.Name), zap.Error(err))
					continue
				}
			}
			middleware.GetLogger().Info("migrated api key to credential",
				zap.String("channel", ch.Name), zap.String("source", cred.Source))
		}

		if ch.BalanceConfigJSON == "" {
			continue
		}
		var balCredCount int64
		if err := db.Model(&LLMChannelCredential{}).Where("channel_id = ? AND purpose = ?", ch.ID, "balance").Count(&balCredCount).Error; err != nil {
			middleware.GetLogger().Warn("count balance credentials failed, skipping channel",
				zap.String("channel", ch.Name), zap.Error(err))
			continue
		}
		if balCredCount > 0 {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(ch.BalanceConfigJSON), &raw); err != nil {
			continue
		}

		secrets := make(map[string]interface{})
		nonSecrets := make(map[string]interface{})
		for k, v := range raw {
			isSecret := false
			for _, sk := range balanceSecretKeys {
				if strings.EqualFold(k, sk) {
					isSecret = true
					break
				}
			}
			if isSecret {
				secrets[k] = v
			} else {
				nonSecrets[k] = v
			}
		}

		if len(secrets) > 0 {
			secretJSON, err := json.Marshal(secrets)
			if err != nil {
				middleware.GetLogger().Warn("marshal balance secrets failed",
					zap.String("channel", ch.Name), zap.Error(err))
				continue
			}
			enc, err := crypto.Encrypt(string(secretJSON))
			if err != nil {
				middleware.GetLogger().Warn("encrypt balance secrets failed",
					zap.String("channel", ch.Name), zap.Error(err))
				continue
			}
			cred := LLMChannelCredential{
				ChannelID:      ch.ID,
				Purpose:        "balance",
				Source:         "direct",
				IsPreset:       true,
				Label:          "balance-secrets",
				CredentialJSON: enc,
				Status:         "active",
			}
			now := time.Now()
			cred.LastRefreshedAt = &now

			err = db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Create(&cred).Error; err != nil {
					return fmt.Errorf("create balance credential: %w", err)
				}
				if len(nonSecrets) > 0 {
					cleanJSON, err := json.Marshal(nonSecrets)
					if err != nil {
						return fmt.Errorf("marshal non-secret balance config: %w", err)
					}
					if err := tx.Model(&LLMChannel{}).Where("id = ?", ch.ID).Update("balance_config_json", string(cleanJSON)).Error; err != nil {
						return fmt.Errorf("update balance_config_json: %w", err)
					}
				} else {
					if err := tx.Model(&LLMChannel{}).Where("id = ?", ch.ID).Update("balance_config_json", "").Error; err != nil {
						return fmt.Errorf("clear balance_config_json: %w", err)
					}
				}
				return nil
			})
			if err != nil {
				middleware.GetLogger().Warn("migrate balance credential transaction failed",
					zap.String("channel", ch.Name), zap.Error(err))
				continue
			}
			middleware.GetLogger().Info("migrated balance secrets to credential",
				zap.String("channel", ch.Name))
		}
	}

	return nil
}
