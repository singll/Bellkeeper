package service

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type SettingService struct {
	repo *repository.SettingRepository
}

func NewSettingService(repo *repository.SettingRepository) *SettingService {
	return &SettingService{repo: repo}
}

func (s *SettingService) List(category string) ([]model.Setting, error) {
	return s.repo.List(category)
}

func (s *SettingService) Get(key string) (*model.Setting, error) {
	return s.repo.GetByKey(key)
}

func (s *SettingService) Set(key, value, valueType, category, description string, isSecret bool) error {
	return s.repo.Set(key, value, valueType, category, description, isSecret)
}

func (s *SettingService) Delete(key string) error {
	return s.repo.Delete(key)
}
