package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatTopicsForPrompt_LessThan30(t *testing.T) {
	topics := []string{"topic A", "topic B", "topic C"}
	result := formatTopicsForPrompt(topics)
	assert.Contains(t, result, "- topic A")
	assert.Contains(t, result, "- topic B")
	assert.Contains(t, result, "- topic C")
	assert.NotContains(t, result, "更多省略")
}

func TestFormatTopicsForPrompt_Exactly30(t *testing.T) {
	topics := make([]string, 30)
	for i := range topics {
		topics[i] = "topic"
	}
	result := formatTopicsForPrompt(topics)
	assert.NotContains(t, result, "更多省略")
}

func TestFormatTopicsForPrompt_Over30(t *testing.T) {
	topics := make([]string, 35)
	for i := range topics {
		topics[i] = "topic"
	}
	result := formatTopicsForPrompt(topics)
	assert.Contains(t, result, "更多省略")
}

func TestFormatTopicsForPrompt_Empty(t *testing.T) {
	result := formatTopicsForPrompt(nil)
	assert.Empty(t, strings.TrimSpace(result))
}

func TestActionStatEntryAggregation_RSIngest(t *testing.T) {
	entries := []ActionStatEntry{
		{Action: "ingest", Status: "success", Count: 100},
		{Action: "ingest", Status: "duplicate", Count: 10},
		{Action: "ingest", Status: "failure", Count: 3},
		{Action: "fetch", Status: "success", Count: 50},
	}

	ingest := &RSSIngestStats{}
	for _, s := range entries {
		if s.Action != "ingest" {
			continue
		}
		switch s.Status {
		case "success":
			ingest.Success = s.Count
		case "duplicate":
			ingest.Duplicate = s.Count
		case "failure":
			ingest.Failure = s.Count
		}
	}

	assert.Equal(t, int64(100), ingest.Success)
	assert.Equal(t, int64(10), ingest.Duplicate)
	assert.Equal(t, int64(3), ingest.Failure)
}

func TestActionStatEntryAggregation_FileIngest(t *testing.T) {
	entries := []ActionStatEntry{
		{Action: "ingest", Status: "success", Count: 20},
		{Action: "ingest", Status: "failure", Count: 2},
	}

	fi := &FileIngestStats{}
	for _, s := range entries {
		switch s.Status {
		case "success":
			fi.Success = s.Count
		case "failure":
			fi.Failure = s.Count
		}
	}

	assert.Equal(t, int64(20), fi.Success)
	assert.Equal(t, int64(2), fi.Failure)
}

func TestActionStatEntryAggregation_Classify(t *testing.T) {
	entries := []ActionStatEntry{
		{Action: "classify", Status: "success", Count: 200},
		{Action: "classify", Status: "failure", Count: 5},
	}

	cl := &ClassifyStats{}
	for _, s := range entries {
		switch s.Status {
		case "success":
			cl.Success = s.Count
		case "failure":
			cl.Failure = s.Count
		}
	}

	assert.Equal(t, int64(200), cl.Success)
	assert.Equal(t, int64(5), cl.Failure)
}

func TestDailyReportData_EmptyCollections(t *testing.T) {
	data := &DailyReportData{
		Date: "2026-06-12",
	}
	md := RenderDailyReport(data)
	assert.Contains(t, md, "2026-06-12 Bellkeeper 日报")
}

func TestDailyReport_CollectError_Structure(t *testing.T) {
	ce := CollectError{Source: "crawl", Error: "timeout"}
	assert.Equal(t, "crawl", ce.Source)
	assert.Equal(t, "timeout", ce.Error)
}

func TestGenerateOptions_Fields(t *testing.T) {
	opts := GenerateOptions{Date: "2026-06-12", DryRun: true, SkipNotify: true}
	assert.Equal(t, "2026-06-12", opts.Date)
	assert.True(t, opts.DryRun)
	assert.True(t, opts.SkipNotify)
}

func TestBriefGenerateOptions_Fields(t *testing.T) {
	opts := BriefGenerateOptions{Date: "2026-06-12"}
	assert.Equal(t, "2026-06-12", opts.Date)
}
