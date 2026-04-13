package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/mmcdole/gofeed"
)

// RSSFetcherConfig holds configuration for the RSS fetcher
type RSSFetcherConfig struct {
	Enabled       bool
	CheckInterval int // seconds between feed checks
	MaxPerBatch   int // max feeds to process per check
	Timeout       int // seconds for each feed fetch
}

// RSSFetcherService handles periodic RSS feed fetching
type RSSFetcherService struct {
	cfg        RSSFetcherConfig
	rssRepo    *repository.RSSRepository
	ingestion  *FileIngestionService
	fetcher    *gofeed.Parser
	stopCh     chan struct{}
	wg         sync.WaitGroup
	running    bool
	mu         sync.RWMutex
}

// NewRSSFetcherService creates a new RSS fetcher service
func NewRSSFetcherService(
	cfg RSSFetcherConfig,
	rssRepo *repository.RSSRepository,
	ingestion *FileIngestionService,
) *RSSFetcherService {
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 60 // default 60 seconds
	}
	if cfg.MaxPerBatch <= 0 {
		cfg.MaxPerBatch = 5 // default 5 feeds per batch
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 // default 30 seconds timeout
	}

	return &RSSFetcherService{
		cfg:       cfg,
		rssRepo:   rssRepo,
		ingestion: ingestion,
		fetcher:   gofeed.NewParser(),
		stopCh:    make(chan struct{}),
	}
}

// Start starts the RSS fetcher background loop
func (s *RSSFetcherService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.runLoop(ctx)
	log.Println("[RSSFetcher] started")
}

// Stop stops the RSS fetcher background loop
func (s *RSSFetcherService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
	log.Println("[RSSFetcher] stopped")
}

// runLoop runs the main fetch loop
func (s *RSSFetcherService) runLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.cfg.CheckInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.fetchAllActive(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// fetchAllActive fetches all active RSS feeds
func (s *RSSFetcherService) fetchAllActive(ctx context.Context) {
	feeds, err := s.rssRepo.GetActive()
	if err != nil {
		log.Printf("[RSSFetcher] failed to get active feeds: %v", err)
		return
	}

	if len(feeds) == 0 {
		return
	}

	log.Printf("[RSSFetcher] fetching %d active feeds", len(feeds))

	// Process feeds in batches
	batchSize := s.cfg.MaxPerBatch
	for i := 0; i < len(feeds); i += batchSize {
		end := i + batchSize
		if end > len(feeds) {
			end = len(feeds)
		}

		batch := feeds[i:end]
		for _, feed := range batch {
			if err := s.fetchFeed(ctx, &feed); err != nil {
				log.Printf("[RSSFetcher] failed to fetch feed %d (%s): %v", feed.ID, feed.Name, err)
			}
		}
	}
}

// FetchFeed fetches a single RSS feed by ID
func (s *RSSFetcherService) FetchFeed(ctx context.Context, feedID uint) error {
	feed, err := s.rssRepo.GetByID(feedID)
	if err != nil {
		return fmt.Errorf("failed to get feed: %w", err)
	}

	return s.fetchFeed(ctx, feed)
}

// fetchFeed fetches and processes a single RSS feed
func (s *RSSFetcherService) fetchFeed(ctx context.Context, feed *model.RSSFeed) error {
	// Set timeout for this fetch
	fetchCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.Timeout)*time.Second)
	defer cancel()

	log.Printf("[RSSFetcher] fetching feed: %s (%s)", feed.Name, feed.URL)

	// Parse RSS feed
	rssFeed, err := s.fetcher.ParseURLWithContext(feed.URL, fetchCtx)
	if err != nil {
		// Update last_fetched_at even on error
		now := time.Now()
		feed.LastFetchedAt = &now
		s.rssRepo.Update(feed)
		return fmt.Errorf("failed to parse RSS: %w", err)
	}

	// Update last_fetched_at
	now := time.Now()
	feed.LastFetchedAt = &now
	if err := s.rssRepo.Update(feed); err != nil {
		log.Printf("[RSSFetcher] failed to update feed timestamp: %v", err)
	}

	if len(rssFeed.Items) == 0 {
		log.Printf("[RSSFetcher] no items in feed %s", feed.Name)
		return nil
	}

	log.Printf("[RSSFetcher] found %d items in feed %s", len(rssFeed.Items), feed.Name)

	// Process each item
	for _, item := range rssFeed.Items {
		if item.Link == "" {
			continue
		}

		// Check if already ingested (skip duplicates)
		if s.ingestion == nil {
			continue
		}

		result, err := s.ingestion.IngestURL(&IngestURLRequest{
			URL:   item.Link,
			Title: item.Title,
		})
		if err != nil {
			log.Printf("[RSSFetcher] failed to ingest %s: %v", item.Link, err)
			continue
		}

		if result.Status == "duplicate" {
			log.Printf("[RSSFetcher] skipped duplicate: %s", item.Link)
		} else if result.Status == "success" {
			log.Printf("[RSSFetcher] ingested: %s -> %s", item.Link, result.FilePath)
		}
	}

	return nil
}

// FetchFeedResult holds the result of a fetch operation
type FetchFeedResult struct {
	FeedID     uint   `json:"feed_id"`
	FeedName   string `json:"feed_name"`
	ItemsFound int    `json:"items_found"`
	ItemsNew   int    `json:"items_new"`
	ItemsDup   int    `json:"items_dup"`
	Error      string `json:"error,omitempty"`
}

// FetchAllResult holds the result of fetching all feeds
type FetchAllResult struct {
	TotalFeeds int              `json:"total_feeds"`
	Results    []FetchFeedResult `json:"results"`
}

// FetchAll fetches all active feeds and returns results
func (s *RSSFetcherService) FetchAll(ctx context.Context) (*FetchAllResult, error) {
	feeds, err := s.rssRepo.GetActive()
	if err != nil {
		return nil, fmt.Errorf("failed to get feeds: %w", err)
	}

	result := &FetchAllResult{
		TotalFeeds: len(feeds),
		Results:    make([]FetchFeedResult, 0, len(feeds)),
	}

	for _, feed := range feeds {
		fetchResult := FetchFeedResult{
			FeedID:   feed.ID,
			FeedName: feed.Name,
		}

		if err := s.fetchFeed(ctx, &feed); err != nil {
			fetchResult.Error = err.Error()
		}

		result.Results = append(result.Results, fetchResult)
	}

	return result, nil
}

// GetStatus returns the current status of the RSS fetcher
func (s *RSSFetcherService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"running": s.running,
		"config":  s.cfg,
	}
}
