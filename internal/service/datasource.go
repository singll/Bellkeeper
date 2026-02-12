package service

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type DataSourceService struct {
	repo    *repository.DataSourceRepository
	tagRepo *repository.TagRepository
}

func NewDataSourceService(repo *repository.DataSourceRepository, tagRepo *repository.TagRepository) *DataSourceService {
	return &DataSourceService{repo: repo, tagRepo: tagRepo}
}

func (s *DataSourceService) List(page, perPage int, category, keyword string) ([]model.DataSource, int64, error) {
	return s.repo.List(page, perPage, category, keyword)
}

func (s *DataSourceService) GetByID(id uint) (*model.DataSource, error) {
	return s.repo.GetByID(id)
}

func (s *DataSourceService) Create(source *model.DataSource, tagIDs []uint) error {
	if err := s.repo.Create(source); err != nil {
		return err
	}

	if len(tagIDs) > 0 {
		tags, err := s.tagRepo.GetByIDs(tagIDs)
		if err != nil {
			return err
		}
		return s.repo.UpdateTags(source, tags)
	}

	return nil
}

func (s *DataSourceService) Update(source *model.DataSource, tagIDs []uint) error {
	if err := s.repo.Update(source); err != nil {
		return err
	}

	tags, err := s.tagRepo.GetByIDs(tagIDs)
	if err != nil {
		return err
	}
	return s.repo.UpdateTags(source, tags)
}

func (s *DataSourceService) Delete(id uint) error {
	return s.repo.Delete(id)
}
