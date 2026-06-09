package service

import (
	"io"
	"strings"
	"testing"
)

func TestStreamUsageTrackerOpenAIUsageTrailer(t *testing.T) {
	tracker := newStreamUsageTracker(io.NopCloser(strings.NewReader(
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":7,\"prompt_tokens_details\":{\"cached_tokens\":3}}}\n\n" +
			"data: [DONE]\n\n",
	)))
	if _, err := io.Copy(io.Discard, tracker); err != nil {
		t.Fatal(err)
	}

	prompt, comp, cached := tracker.Usage()
	if prompt != 12 || comp != 7 || cached != 3 {
		t.Fatalf("usage = %d/%d/%d, want 12/7/3", prompt, comp, cached)
	}
}

func TestStreamUsageTrackerAnthropicUsageEvents(t *testing.T) {
	tracker := newStreamUsageTracker(io.NopCloser(strings.NewReader(
		"event: message_start\n" +
			"data: {\"usage\":{\"input_tokens\":5,\"cache_read_input_tokens\":2}}\n\n" +
			"event: message_delta\n" +
			"data: {\"usage\":{\"output_tokens\":9}}\n\n",
	)))
	if _, err := io.Copy(io.Discard, tracker); err != nil {
		t.Fatal(err)
	}

	prompt, comp, cached := tracker.Usage()
	if prompt != 7 || comp != 9 || cached != 2 {
		t.Fatalf("usage = %d/%d/%d, want 7/9/2", prompt, comp, cached)
	}
}
