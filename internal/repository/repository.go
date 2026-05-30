package repository

import (
	"gorm.io/gorm"
)

// Repositories holds all repository instances
type Repositories struct {
	Tag            *TagRepository
	RSS            *RSSRepository
	DatasetMapping *DatasetMappingRepository
	Setting        *SettingRepository
	LLMProxy       *LLMProxyRepository
	LLMChannel     *LLMChannelRepository
	LLMModelGroup  *LLMModelGroupRepository
	LLMToken       *LLMTokenRepository
	LLMTokenUsage  *LLMTokenUsageRepository
	LLMModelPricing *LLMModelPricingRepository
	LLMRateLimit            *LLMRateLimitRepository
	LLMConversationBinding  *ConversationBindingRepository
	ActivityLog    *ActivityLogRepository
	LogSource      *LogSourceRepository
	LogEntry       *LogEntryRepository
	LogAlertRule   *LogAlertRuleRepository
	ArticleTag     *ArticleTagRepository
	// Matrix Platform
	MatrixRoom         *MatrixRoomRepository
	MatrixChannel      *MatrixChannelRepository
	MatrixCommand      *MatrixCommandRepository
	MatrixEvent        *MatrixEventRepository
	MatrixNotification *MatrixNotificationRepository
	MatrixCommandLog   *MatrixCommandLogRepository
	MatrixSyncState    *MatrixSyncStateRepository
	MatrixUserRole     *MatrixUserRoleRepository
	CrawlJob           *CrawlJobRepository
}

// NewRepositories creates all repository instances
func NewRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		Tag:            NewTagRepository(db),
		RSS:            NewRSSRepository(db),
		DatasetMapping: NewDatasetMappingRepository(db),
		Setting:        NewSettingRepository(db),
		LLMProxy:        NewLLMProxyRepository(db),
		LLMChannel:      NewLLMChannelRepository(db),
		LLMModelGroup:   NewLLMModelGroupRepository(db),
		LLMToken:        NewLLMTokenRepository(db),
		LLMTokenUsage:   NewLLMTokenUsageRepository(db),
		LLMModelPricing: NewLLMModelPricingRepository(db),
		LLMRateLimit:            NewLLMRateLimitRepository(db),
		LLMConversationBinding:  NewConversationBindingRepository(db),
		ActivityLog:    NewActivityLogRepository(db),
		LogSource:      NewLogSourceRepository(db),
		LogEntry:       NewLogEntryRepository(db),
		LogAlertRule:   NewLogAlertRuleRepository(db),
		ArticleTag:     NewArticleTagRepository(db),
		// Matrix Platform
		MatrixRoom:         NewMatrixRoomRepository(db),
		MatrixChannel:      NewMatrixChannelRepository(db),
		MatrixCommand:      NewMatrixCommandRepository(db),
		MatrixEvent:        NewMatrixEventRepository(db),
		MatrixNotification: NewMatrixNotificationRepository(db),
		MatrixCommandLog:   NewMatrixCommandLogRepository(db),
		MatrixSyncState:    NewMatrixSyncStateRepository(db),
		MatrixUserRole:     NewMatrixUserRoleRepository(db),
		CrawlJob:           NewCrawlJobRepository(db),
	}
}
