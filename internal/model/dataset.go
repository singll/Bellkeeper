package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DatasetMapping represents a mapping between tags and RagFlow datasets
// and can also act as a routing rule for file-first document ingestion.
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

// ArticleTag represents the association between an article and tags.
// It also serves as the local metadata ledger for file-first ingestion.
type ArticleTag struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	DocumentID     string         `gorm:"size:100;not null;index" json:"document_id"`
	DatasetID      string         `gorm:"size:100;not null;index" json:"dataset_id"`
	TagID          uint           `gorm:"index" json:"tag_id"`
	ArticleTitle   string         `gorm:"size:1000" json:"article_title"`
	ArticleURL     string         `gorm:"size:2000;index" json:"article_url"`
	CanonicalURL   string         `gorm:"size:2000" json:"canonical_url,omitempty"`
	ContentHash    string         `gorm:"size:128;index" json:"content_hash,omitempty"`
	SourceDomain   string         `gorm:"size:255;index" json:"source_domain,omitempty"`
	FilePath       string         `gorm:"size:2000;index" json:"file_path,omitempty"`
	Layer          string         `gorm:"size:50;index" json:"layer,omitempty"`
	Extractor      string         `gorm:"size:100" json:"extractor,omitempty"`
	IngestStatus   string         `gorm:"size:50;default:'ingested';index" json:"ingest_status,omitempty"`
	IndexStatus    string         `gorm:"size:50;default:'pending';index" json:"index_status,omitempty"`
	Metadata       datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	Tag Tag `gorm:"foreignKey:TagID" json:"tag,omitempty"`
}

// TableName specifies table name
func (ArticleTag) TableName() string {
	return "article_tags"
}
