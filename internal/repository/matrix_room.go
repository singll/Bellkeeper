package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
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
	// Ensure Config is valid JSON (empty string causes PostgreSQL jsonb error)
	if room.Config == "" {
		room.Config = "{}"
	}
	return r.db.Create(room).Error
}

func (r *MatrixRoomRepository) Update(room *model.MatrixRoom) error {
	return r.db.Save(room).Error
}
