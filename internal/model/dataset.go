package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DatasetMapping represents a mapping between tags and RagFlow datasets
type DatasetMapping struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:100;uniqueIndex;not null" json:"name"`
	DisplayName string         `gorm:"size:200" json:"display_name"`
	DatasetID   string         `gorm:"size:100;not null" json:"dataset_id"`
	Description string         `gorm:"type:text" json:"description"`
	IsDefault   bool           `gorm:"default:false" json:"is_default"`
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	ParserID    string         `gorm:"size:50;default:'naive'" json:"parser_id"`
	Metadata    datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	Tags []Tag `gorm:"many2many:dataset_mapping_tags;" json:"tags,omitempty"`
}

// TableName specifies table name
func (DatasetMapping) TableName() string {
	return "dataset_mappings"
}

// ArticleTag represents the association between an article and tags
type ArticleTag struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	DocumentID   string    `gorm:"size:100;not null;index" json:"document_id"`
	DatasetID    string    `gorm:"size:100;not null;index" json:"dataset_id"`
	TagID        uint      `gorm:"index" json:"tag_id"`
	ArticleTitle string    `gorm:"size:1000" json:"article_title"`
	ArticleURL   string    `gorm:"size:2000" json:"article_url"`
	CreatedAt    time.Time `json:"created_at"`

	// Relations
	Tag Tag `gorm:"foreignKey:TagID" json:"tag,omitempty"`
}

// TableName specifies table name
func (ArticleTag) TableName() string {
	return "article_tags"
}
