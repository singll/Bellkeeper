package repository

import (
	"context"
	"errors"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

// MatrixUserRoleRepository handles matrix_user_roles table operations
type MatrixUserRoleRepository struct {
	db *gorm.DB
}

// NewMatrixUserRoleRepository creates a new repository
func NewMatrixUserRoleRepository(db *gorm.DB) *MatrixUserRoleRepository {
	return &MatrixUserRoleRepository{db: db}
}

// GetByUserAndRoom gets a user's role in a specific room
func (r *MatrixUserRoleRepository) GetByUserAndRoom(ctx context.Context, userID, roomID string) (*model.MatrixUserRole, error) {
	var userRole model.MatrixUserRole
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND room_id = ?", userID, roomID).
		First(&userRole).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &userRole, nil
}

// GetByUser gets all roles for a user across all rooms
func (r *MatrixUserRoleRepository) GetByUser(ctx context.Context, userID string) ([]*model.MatrixUserRole, error) {
	var roles []*model.MatrixUserRole
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// GetByRoom gets all user roles in a room
func (r *MatrixUserRoleRepository) GetByRoom(ctx context.Context, roomID string) ([]*model.MatrixUserRole, error) {
	var roles []*model.MatrixUserRole
	err := r.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// Upsert creates or updates a user role
func (r *MatrixUserRoleRepository) Upsert(ctx context.Context, userID, roomID, role string) error {
	userRole := &model.MatrixUserRole{
		UserID: userID,
		RoomID: roomID,
		Role:   role,
	}

	return r.db.WithContext(ctx).
		Where("user_id = ? AND room_id = ?", userID, roomID).
		Assign(userRole).
		FirstOrCreate(userRole).Error
}

// Delete removes a user's role from a room
func (r *MatrixUserRoleRepository) Delete(ctx context.Context, userID, roomID string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND room_id = ?", userID, roomID).
		Delete(&model.MatrixUserRole{}).Error
}

// DeleteByUser removes all roles for a user
func (r *MatrixUserRoleRepository) DeleteByUser(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.MatrixUserRole{}).Error
}

// List lists all user roles
func (r *MatrixUserRoleRepository) List(ctx context.Context, limit, offset int) ([]*model.MatrixUserRole, error) {
	var roles []*model.MatrixUserRole
	query := r.db.WithContext(ctx)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// Count counts all user roles
func (r *MatrixUserRoleRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.MatrixUserRole{}).Count(&count).Error
	return count, err
}
