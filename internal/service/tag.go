package service

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type TagService struct {
	repo *repository.TagRepository
}

func NewTagService(repo *repository.TagRepository) *TagService {
	return &TagService{repo: repo}
}

func (s *TagService) List(page, perPage int, keyword string) ([]model.Tag, int64, error) {
	return s.repo.List(page, perPage, keyword)
}

func (s *TagService) GetByID(id uint) (*model.Tag, error) {
	return s.repo.GetByID(id)
}

func (s *TagService) Create(tag *model.Tag) error {
	return s.repo.Create(tag)
}

func (s *TagService) Update(tag *model.Tag) error {
	return s.repo.Update(tag)
}

func (s *TagService) Delete(id uint) error {
	return s.repo.Delete(id)
}

func (s *TagService) GetOrCreateByNames(names []string) ([]model.Tag, error) {
	var tags []model.Tag
	for _, name := range names {
		tag, err := s.repo.GetByName(name)
		if err != nil {
			// Create new tag
			tag = &model.Tag{Name: name, Color: "#409EFF"}
			if err := s.repo.Create(tag); err != nil {
				return nil, err
			}
		}
		tags = append(tags, *tag)
	}
	return tags, nil
}
