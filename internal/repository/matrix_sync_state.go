package repository

import (
	"context"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type MatrixSyncStateRepository struct {
	db *gorm.DB
}

func NewMatrixSyncStateRepository(db *gorm.DB) *MatrixSyncStateRepository {
	return &MatrixSyncStateRepository{db: db}
}

func (r *MatrixSyncStateRepository) Get(botUserID string) (*model.MatrixSyncState, error) {
	var state model.MatrixSyncState
	if err := r.db.Where("bot_user_id = ?", botUserID).First(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *MatrixSyncStateRepository) GetByUserID(ctx context.Context, botUserID string) (*model.MatrixSyncState, error) {
	var state model.MatrixSyncState
	if err := r.db.WithContext(ctx).Where("bot_user_id = ?", botUserID).First(&state).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

func (r *MatrixSyncStateRepository) Upsert(ctx context.Context, state *model.MatrixSyncState) error {
	state.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Where("bot_user_id = ?", state.BotUserID).Assign(state).FirstOrCreate(state).Error
}
