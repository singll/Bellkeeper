package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

// LLMChannelBalanceSnapshotRepository persists periodic balance snapshots and serves
// the per-channel balance history endpoint.
type LLMChannelBalanceSnapshotRepository struct {
	db *gorm.DB
}

func NewLLMChannelBalanceSnapshotRepository(db *gorm.DB) *LLMChannelBalanceSnapshotRepository {
	return &LLMChannelBalanceSnapshotRepository{db: db}
}

// Create inserts a new snapshot row.
func (r *LLMChannelBalanceSnapshotRepository) Create(s *model.LLMChannelBalanceSnapshot) error {
	return r.db.Create(s).Error
}

// ListByChannelName returns recent snapshots for a channel, newest first, capped at limit.
func (r *LLMChannelBalanceSnapshotRepository) ListByChannelName(name string, limit int) ([]model.LLMChannelBalanceSnapshot, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var snaps []model.LLMChannelBalanceSnapshot
	err := r.db.Where("channel_name = ?", name).Order("fetched_at DESC").Limit(limit).Find(&snaps).Error
	return snaps, err
}
