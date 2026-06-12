package repository

import (
	"context"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MatrixRoomRepository struct {
	db *gorm.DB
}

func NewMatrixRoomRepository(db *gorm.DB) *MatrixRoomRepository {
	return &MatrixRoomRepository{db: db}
}

func (r *MatrixRoomRepository) List(activeOnly bool) ([]model.MatrixRoom, error) {
	var rooms []model.MatrixRoom
	query := r.db.Model(&model.MatrixRoom{})
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	if err := query.Order("id ASC").Find(&rooms).Error; err != nil {
		return nil, err
	}
	return rooms, nil
}

func (r *MatrixRoomRepository) GetByRoomID(roomID string) (*model.MatrixRoom, error) {
	var room model.MatrixRoom
	if err := r.db.Where("room_id = ?", roomID).First(&room).Error; err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *MatrixRoomRepository) Create(room *model.MatrixRoom) error {
	if room.Config == nil {
		s := "{}"
		room.Config = &s
	}
	return r.db.Create(room).Error
}

func (r *MatrixRoomRepository) Update(room *model.MatrixRoom) error {
	return r.db.Save(room).Error
}

func (r *MatrixRoomRepository) Delete(roomID string) error {
	return r.db.Where("room_id = ?", roomID).Delete(&model.MatrixRoom{}).Error
}

func (r *MatrixRoomRepository) Upsert(ctx context.Context, room *model.MatrixRoom) error {
	room.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "room_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"room_name", "room_type", "is_active", "updated_at"}),
	}).Create(room).Error
}
