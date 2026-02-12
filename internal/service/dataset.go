package service

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type DatasetService struct {
	repo    *repository.DatasetMappingRepository
	tagRepo *repository.TagRepository
}

func NewDatasetService(repo *repository.DatasetMappingRepository, tagRepo *repository.TagRepository) *DatasetService {
	return &DatasetService{repo: repo, tagRepo: tagRepo}
}

func (s *DatasetService) List(page, perPage int) ([]model.DatasetMapping, int64, error) {
	return s.repo.List(page, perPage)
}

func (s *DatasetService) GetByID(id uint) (*model.DatasetMapping, error) {
	return s.repo.GetByID(id)
}

func (s *DatasetService) GetByName(name string) (*model.DatasetMapping, error) {
	return s.repo.GetByName(name)
}

func (s *DatasetService) GetDefault() (*model.DatasetMapping, error) {
	return s.repo.GetDefault()
}

func (s *DatasetService) Create(mapping *model.DatasetMapping, tagIDs []uint) error {
	if err := s.repo.Create(mapping); err != nil {
		return err
	}

	if len(tagIDs) > 0 {
		tags, err := s.tagRepo.GetByIDs(tagIDs)
		if err != nil {
			return err
		}
		return s.repo.UpdateTags(mapping, tags)
	}

	return nil
}

func (s *DatasetService) Update(mapping *model.DatasetMapping, tagIDs []uint) error {
	if err := s.repo.Update(mapping); err != nil {
		return err
	}

	tags, err := s.tagRepo.GetByIDs(tagIDs)
	if err != nil {
		return err
	}
	return s.repo.UpdateTags(mapping, tags)
}

func (s *DatasetService) Delete(id uint) error {
	return s.repo.Delete(id)
}

// GetByTagIDs finds dataset mappings that match any of the given tag IDs
func (s *DatasetService) GetByTagIDs(tagIDs []uint) ([]model.DatasetMapping, error) {
	return s.repo.GetByTagIDs(tagIDs)
}
