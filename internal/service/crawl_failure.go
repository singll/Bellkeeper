package service

import (
	"fmt"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type CrawlFailureService struct {
	failureRepo *repository.CrawlFailureRepository
	jobRepo     *repository.CrawlJobRepository
}

func NewCrawlFailureService(
	failureRepo *repository.CrawlFailureRepository,
	jobRepo *repository.CrawlJobRepository,
) *CrawlFailureService {
	return &CrawlFailureService{
		failureRepo: failureRepo,
		jobRepo:     jobRepo,
	}
}

func (s *CrawlFailureService) List(opts repository.ListCrawlFailuresOpts) ([]model.CrawlFailure, int64, error) {
	return s.failureRepo.List(opts)
}

func (s *CrawlFailureService) GetByID(id uint) (*model.CrawlFailure, error) {
	return s.failureRepo.GetByID(id)
}

func (s *CrawlFailureService) Retry(id uint) error {
	failure, err := s.failureRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("lookup crawl failure %d: %w", id, err)
	}
	if failure == nil {
		return fmt.Errorf("crawl failure %d not found", id)
	}
	if failure.Status == model.CrawlFailureAbandoned {
		return fmt.Errorf("cannot retry abandoned failure %d", id)
	}

	if s.jobRepo == nil {
		return fmt.Errorf("crawl failure retry: job repository not configured")
	}
	job := &model.CrawlJob{
		SourceID:     failure.SourceID,
		URL:          failure.URL,
		Title:        failure.Title,
		Status:       model.CrawlJobPending,
		ChannelType:  "auto",
		SourceDomain: failure.SourceDomain,
		MaxRetries:   3,
	}
	if err := s.jobRepo.Enqueue(job); err != nil {
		return fmt.Errorf("enqueue retry job for failure %d: %w", id, err)
	}

	if err := s.failureRepo.MarkRecoveryAttempt(id); err != nil {
		return fmt.Errorf("mark recovery attempt for failure %d: %w", id, err)
	}
	return nil
}

func (s *CrawlFailureService) Abandon(id uint) error {
	return s.failureRepo.MarkAbandoned(id)
}
