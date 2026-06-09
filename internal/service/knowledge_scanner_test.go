package service

import "testing"

func TestGetTagsFromMapInlineArray(t *testing.T) {
	got := getTagsFromMap(map[string]string{"tags": "[go, pkb]"})
	want := []string{"go", "pkb"}
	if len(got) != len(want) {
		t.Fatalf("tags len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tags[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseSimpleYAMLListTags(t *testing.T) {
	fm := parseSimpleYAML("title: Test\ntags:\n  - go\n  - pkb\n")
	got := getTagsFromMap(fm)
	want := []string{"go", "pkb"}
	if len(got) != len(want) {
		t.Fatalf("tags len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tags[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
