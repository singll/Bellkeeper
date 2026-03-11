package model

import (
	"time"

	"gorm.io/gorm"
)

// LLMModelGroup represents a virtual model group that routes to multiple channels.
type LLMModelGroup struct {
	ID               uint                  `gorm:"primaryKey" json:"id"`
	Name             string                `gorm:"size:100;uniqueIndex;not null" json:"name"`
	Description      string                `gorm:"size:500" json:"description"`
	Strategy         string                `gorm:"size:50;default:'priority-health'" json:"strategy"`
	StickyTTLSeconds int                   `gorm:"default:600" json:"sticky_ttl_seconds"`
	Members          []LLMModelGroupMember `gorm:"foreignKey:GroupID" json:"members"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	DeletedAt        gorm.DeletedAt        `gorm:"index" json:"-"`
}

func (LLMModelGroup) TableName() string {
	return "llm_model_groups"
}

// LLMModelGroupMember represents a member channel within a model group.
type LLMModelGroupMember struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	GroupID     uint           `gorm:"index;not null" json:"group_id"`
	ChannelName string         `gorm:"size:100;not null" json:"channel_name"`
	Model       string         `gorm:"size:200;not null" json:"model"`
	Weight      int            `gorm:"default:1" json:"weight"`
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (LLMModelGroupMember) TableName() string {
	return "llm_model_group_members"
}
