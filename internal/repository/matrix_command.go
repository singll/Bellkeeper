package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type MatrixCommandRepository struct {
	db *gorm.DB
}

func NewMatrixCommandRepository(db *gorm.DB) *MatrixCommandRepository {
	return &MatrixCommandRepository{db: db}
}

func (r *MatrixCommandRepository) List(activeOnly bool) ([]model.MatrixCommand, error) {
	var commands []model.MatrixCommand
	query := r.db.Model(&model.MatrixCommand{})
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	if err := query.Order("command_name ASC").Find(&commands).Error; err != nil {
		return nil, err
	}
	return commands, nil
}

func (r *MatrixCommandRepository) GetByName(name string) (*model.MatrixCommand, error) {
	var cmd model.MatrixCommand
	if err := r.db.Where("command_name = ? AND is_active = ?", name, true).First(&cmd).Error; err != nil {
		return nil, err
	}
	return &cmd, nil
}

func (r *MatrixCommandRepository) Create(cmd *model.MatrixCommand) error {
	if cmd.HandlerConfig == nil {
		s := "{}"
		cmd.HandlerConfig = &s
	}
	return r.db.Create(cmd).Error
}

func (r *MatrixCommandRepository) Update(cmd *model.MatrixCommand) error {
	return r.db.Save(cmd).Error
}
