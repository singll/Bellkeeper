package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type MatrixChannelRepository struct {
	db *gorm.DB
}

func NewMatrixChannelRepository(db *gorm.DB) *MatrixChannelRepository {
	return &MatrixChannelRepository{db: db}
}

func (r *MatrixChannelRepository) List(activeOnly bool) ([]model.MatrixChannel, error) {
	var channels []model.MatrixChannel
	query := r.db.Model(&model.MatrixChannel{})
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	if err := query.Order("priority DESC, id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

func (r *MatrixChannelRepository) GetByName(name string) (*model.MatrixChannel, error) {
	var channel model.MatrixChannel
	if err := r.db.Where("channel_name = ?", name).First(&channel).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

func (r *MatrixChannelRepository) Create(channel *model.MatrixChannel) error {
	return r.db.Create(channel).Error
}

func (r *MatrixChannelRepository) Update(channel *model.MatrixChannel) error {
	return r.db.Save(channel).Error
}
