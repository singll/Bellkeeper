package service

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
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
			tag = &model.Tag{Name: name, Color: defaults.DefaultTagColor}
			if err := s.repo.Create(tag); err != nil {
				return nil, err
			}
		}
		tags = append(tags, *tag)
	}
	return tags, nil
}

func (s *TagService) GetAll() ([]model.Tag, error) {
	return s.repo.GetAll()
}

func (s *TagService) GetByNames(names []string) ([]model.Tag, error) {
	return s.repo.GetByNames(names)
}

func (s *TagService) MatchByKeywords(keywords []string) ([]model.Tag, error) {
	tagMap := make(map[uint]model.Tag)
	for _, kw := range keywords {
		matched, err := s.repo.MatchByKeyword(kw)
		if err != nil {
			return nil, err
		}
		for _, t := range matched {
			tagMap[t.ID] = t
		}
	}
	tags := make([]model.Tag, 0, len(tagMap))
	for _, t := range tagMap {
		tags = append(tags, t)
	}
	return tags, nil
}

// BatchGetOrCreate gets existing tags by name and optionally creates missing ones
func (s *TagService) BatchGetOrCreate(names []string, autoCreate bool) (found []model.Tag, created []model.Tag, notFound []string, err error) {
	existing, err := s.repo.GetByNames(names)
	if err != nil {
		return nil, nil, nil, err
	}

	existingMap := make(map[string]model.Tag)
	for _, t := range existing {
		existingMap[t.Name] = t
	}

	for _, name := range names {
		if t, ok := existingMap[name]; ok {
			found = append(found, t)
		} else if autoCreate {
			tag := &model.Tag{Name: name, Color: defaults.DefaultTagColor}
			if err := s.repo.Create(tag); err != nil {
				return nil, nil, nil, err
			}
			created = append(created, *tag)
		} else {
			notFound = append(notFound, name)
		}
	}
	return found, created, notFound, nil
}
