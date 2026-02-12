package repository

import (
	"gorm.io/gorm"
)

// Repositories holds all repository instances
type Repositories struct {
	Tag            *TagRepository
	DataSource     *DataSourceRepository
	RSS            *RSSRepository
	Webhook        *WebhookRepository
	DatasetMapping *DatasetMappingRepository
	Setting        *SettingRepository
}

// NewRepositories creates all repository instances
func NewRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		Tag:            NewTagRepository(db),
		DataSource:     NewDataSourceRepository(db),
		RSS:            NewRSSRepository(db),
		Webhook:        NewWebhookRepository(db),
		DatasetMapping: NewDatasetMappingRepository(db),
		Setting:        NewSettingRepository(db),
	}
}
