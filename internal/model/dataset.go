package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DatasetMapping is a routing-rule record used by file-first document ingestion:
// it groups tags into a logical "dataset" (Meilisearch partition / storage hint).
type DatasetMapping struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:100;uniqueIndex;not null" json:"name"`
	DisplayName string         `gorm:"size:200" json:"display_name"`
	DatasetID   string         `gorm:"size:100;not null" json:"dataset_id"`
	Description string         `gorm:"type:text" json:"description"`
	IsDefault   bool           `gorm:"default:false" json:"is_default"`
	IsActive    bool           `gorm:"default:true" json:"is_active"`
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
	ID           uint   `gorm:"primaryKey" json:"id"`
	DocumentID   string `gorm:"size:100;not null;index" json:"document_id"`
	DatasetID    string `gorm:"size:100;not null;index" json:"dataset_id"`
	TagID        uint   `gorm:"index" json:"tag_id"`
	ArticleTitle string `gorm:"size:1000" json:"article_title"`
	ArticleURL   string `gorm:"size:2000;index" json:"article_url"`
	CanonicalURL string `gorm:"size:2000" json:"canonical_url,omitempty"`
	ContentHash  string `gorm:"size:128;index" json:"content_hash,omitempty"`
	SourceDomain string `gorm:"size:255;index" json:"source_domain,omitempty"`
	FilePath     string `gorm:"size:2000;index" json:"file_path,omitempty"`
	Layer        string `gorm:"size:50;index" json:"layer,omitempty"`
	Extractor    string `gorm:"size:100" json:"extractor,omitempty"`
	IngestStatus string `gorm:"size:50;default:'ingested';index" json:"ingest_status,omitempty"`
	IndexStatus  string `gorm:"size:50;default:'pending';index" json:"index_status,omitempty"`
	// pkb-curate 处理状态（漏斗幂等：ListRaw 默认排除已处理；账本随文件移动而更新）
	PkbStatus   string         `gorm:"size:50;index" json:"pkb_status,omitempty"` // ""=未处理 / "processed"
	PkbDecision string         `gorm:"size:50" json:"pkb_decision,omitempty"`     // vault / archive / discard
	PkbScore    float64        `gorm:"default:0" json:"pkb_score,omitempty"`      // 最终加权分
	Metadata    datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	Tag Tag `gorm:"foreignKey:TagID" json:"tag,omitempty"`
}

// TableName specifies table name
func (ArticleTag) TableName() string {
	return "article_tags"
}
