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
	ActivityLog    *ActivityLogRepository
	ArticleTag     *ArticleTagRepository
}

// NewRepositories creates all repository instances
func NewRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		Tag:            NewTagRepository(db),
		RSS:            NewRSSRepository(db),
		DatasetMapping: NewDatasetMappingRepository(db),
		Setting:        NewSettingRepository(db),
		LLMProxy:       NewLLMProxyRepository(db),
		LLMChannel:     NewLLMChannelRepository(db),
		LLMModelGroup:  NewLLMModelGroupRepository(db),
		ActivityLog:    NewActivityLogRepository(db),
		ArticleTag:     NewArticleTagRepository(db),
	}
}
