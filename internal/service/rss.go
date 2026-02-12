package service

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type RSSService struct {
	repo    *repository.RSSRepository
	tagRepo *repository.TagRepository
}

func NewRSSService(repo *repository.RSSRepository, tagRepo *repository.TagRepository) *RSSService {
	return &RSSService{repo: repo, tagRepo: tagRepo}
}

func (s *RSSService) List(page, perPage int, category, keyword string) ([]model.RSSFeed, int64, error) {
	return s.repo.List(page, perPage, category, keyword)
}

func (s *RSSService) GetByID(id uint) (*model.RSSFeed, error) {
	return s.repo.GetByID(id)
}

func (s *RSSService) Create(feed *model.RSSFeed, tagIDs []uint) error {
	if err := s.repo.Create(feed); err != nil {
		return err
	}

	if len(tagIDs) > 0 {
		tags, err := s.tagRepo.GetByIDs(tagIDs)
		if err != nil {
			return err
		}
		return s.repo.UpdateTags(feed, tags)
	}

	return nil
}

func (s *RSSService) Update(feed *model.RSSFeed, tagIDs []uint) error {
	if err := s.repo.Update(feed); err != nil {
		return err
	}

	tags, err := s.tagRepo.GetByIDs(tagIDs)
	if err != nil {
		return err
	}
	return s.repo.UpdateTags(feed, tags)
}

func (s *RSSService) Delete(id uint) error {
	return s.repo.Delete(id)
}

func (s *RSSService) GetActive() ([]model.RSSFeed, error) {
	return s.repo.GetActive()
}
