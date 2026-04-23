package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type LogAlertRuleRepository struct {
	db *gorm.DB
}

func NewLogAlertRuleRepository(db *gorm.DB) *LogAlertRuleRepository {
	return &LogAlertRuleRepository{db: db}
}

func (r *LogAlertRuleRepository) Create(rule *model.LogAlertRule) error {
	return r.db.Create(rule).Error
}

func (r *LogAlertRuleRepository) GetByID(id uint) (*model.LogAlertRule, error) {
	var rule model.LogAlertRule
	if err := r.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *LogAlertRuleRepository) List() ([]model.LogAlertRule, error) {
	var rules []model.LogAlertRule
	if err := r.db.Order("id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *LogAlertRuleRepository) ListActive() ([]model.LogAlertRule, error) {
	var rules []model.LogAlertRule
	if err := r.db.Where("is_active = ?", true).Order("id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *LogAlertRuleRepository) Update(rule *model.LogAlertRule) error {
	return r.db.Save(rule).Error
}

func (r *LogAlertRuleRepository) Delete(id uint) error {
	return r.db.Delete(&model.LogAlertRule{}, id).Error
}