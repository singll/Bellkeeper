package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type LLMModelGroupRepository struct {
	db *gorm.DB
}

func NewLLMModelGroupRepository(db *gorm.DB) *LLMModelGroupRepository {
	return &LLMModelGroupRepository{db: db}
}

func (r *LLMModelGroupRepository) List() ([]model.LLMModelGroup, error) {
	var groups []model.LLMModelGroup
	if err := r.db.Preload("Members").Order("name ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *LLMModelGroupRepository) Get(id uint) (*model.LLMModelGroup, error) {
	var group model.LLMModelGroup
	if err := r.db.Preload("Members").First(&group, id).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *LLMModelGroupRepository) Create(g *model.LLMModelGroup) error {
	return r.db.Create(g).Error
}

// Update replaces the group and its members in a transaction.
func (r *LLMModelGroupRepository) Update(g *model.LLMModelGroup) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete old members
		if err := tx.Where("group_id = ?", g.ID).Delete(&model.LLMModelGroupMember{}).Error; err != nil {
			return err
		}
		// Save group (updates fields + creates new members)
		return tx.Save(g).Error
	})
}

func (r *LLMModelGroupRepository) Delete(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete members first
		if err := tx.Where("group_id = ?", id).Delete(&model.LLMModelGroupMember{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.LLMModelGroup{}, id).Error
	})
}
