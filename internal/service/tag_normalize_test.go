package service

import "testing"

func TestNormalizeAutoTagList(t *testing.T) {
	got := normalizeTagList([]string{
		" AI LLM ",
		"[Large-Language-Models]",
		"news",
		"Retrieval Augmented Generation",
		"AI LLM",
	})
	want := []string{"ai-llm", "llm", "news", "retrieval-augmented-generation"}
	if len(got) != len(want) {
		t.Fatalf("tags len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tags[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	got = normalizeAutoTagList([]string{"news", "AI LLM", "article"})
	want = []string{"ai-llm"}
	if len(got) != len(want) {
		t.Fatalf("auto tags len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("auto tags[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
