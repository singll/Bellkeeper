package service

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestRSSFetcherConfig_Defaults(t *testing.T) {
	// Test that default config values are reasonable
	cfg := RSSFetcherConfig{}

	// Before setting defaults, all should be zero
	assert.Equal(t, 0, cfg.CheckInterval)
	assert.Equal(t, 0, cfg.MaxPerBatch)
	assert.Equal(t, 0, cfg.Timeout)
}

func TestRSSFetcherConfig_WithDefaults(t *testing.T) {
	// Simulate the defaults applied in NewRSSFetcherService
	cfg := RSSFetcherConfig{}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 60
	}
	if cfg.MaxPerBatch <= 0 {
		cfg.MaxPerBatch = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}

	assert.Equal(t, 60, cfg.CheckInterval)
	assert.Equal(t, 5, cfg.MaxPerBatch)
	assert.Equal(t, 30, cfg.Timeout)
}

func TestFetchFeedResult_Structure(t *testing.T) {
	result := &FetchFeedResult{
		FeedID:     1,
		FeedName:   "Test Feed",
		ItemsFound: 10,
		ItemsNew:   5,
		ItemsDup:   5,
	}

	assert.Equal(t, uint(1), result.FeedID)
	assert.Equal(t, "Test Feed", result.FeedName)
	assert.Equal(t, 10, result.ItemsFound)
	assert.Equal(t, 5, result.ItemsNew)
	assert.Equal(t, 5, result.ItemsDup)
	assert.Empty(t, result.Error)
}

func TestFetchFeedResult_WithError(t *testing.T) {
	result := &FetchFeedResult{
		FeedID:     1,
		FeedName:   "Test Feed",
		ItemsFound: 0,
		ItemsNew:   0,
		ItemsDup:   0,
		Error:      "connection timeout",
	}

	assert.Equal(t, "connection timeout", result.Error)
}

func TestFetchAllResult_Structure(t *testing.T) {
	result := &FetchAllResult{
		TotalFeeds: 2,
		Results: []FetchFeedResult{
			{FeedID: 1, FeedName: "Feed 1", ItemsFound: 10},
			{FeedID: 2, FeedName: "Feed 2", Error: "failed"},
		},
	}

	assert.Equal(t, 2, result.TotalFeeds)
	assert.Len(t, result.Results, 2)
	assert.Equal(t, "Feed 1", result.Results[0].FeedName)
	assert.Empty(t, result.Results[0].Error)
	assert.Equal(t, "failed", result.Results[1].Error)
}

func TestRSSFetcherService_GetStatus(t *testing.T) {
	// Test GetStatus returns expected structure
	cfg := RSSFetcherConfig{
		Enabled:       true,
		CheckInterval: 60,
		MaxPerBatch:   5,
		Timeout:       30,
	}

	// Create a mock service for status testing
	svc := &RSSFetcherService{
		cfg: cfg,
	}

	status := svc.GetStatus()
	assert.NotNil(t, status)
	assert.Equal(t, false, status["running"]) // Not started yet
	assert.Equal(t, cfg, status["config"])
}

func TestRSSFeed_Integration(t *testing.T) {
	// Test RSSFeed model structure
	feed := &model.RSSFeed{
		ID:                   1,
		Name:                 "Test RSS Feed",
		URL:                  "https://example.com/feed.xml",
		Category:             "tech",
		IsActive:             true,
		FetchIntervalMinutes: 60,
	}

	assert.Equal(t, uint(1), feed.ID)
	assert.Equal(t, "Test RSS Feed", feed.Name)
	assert.Equal(t, "https://example.com/feed.xml", feed.URL)
	assert.Equal(t, "tech", feed.Category)
	assert.True(t, feed.IsActive)
	assert.Equal(t, 60, feed.FetchIntervalMinutes)
}

func TestRSSFeed_TableName(t *testing.T) {
	feed := model.RSSFeed{}
	assert.Equal(t, "rss_feeds", feed.TableName())
}
