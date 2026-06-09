package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/singll/bellkeeper/internal/config"
)

func TestKnowledgeFilesServiceRejectsParentTraversal(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "knowledge")
	outside := filepath.Join(root, "knowledge2")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewKnowledgeFilesService(config.KnowledgeConfig{BasePath: base})
	_, err := svc.ReadFile(filepath.Join("..", "knowledge2", "secret.md"))
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("ReadFile traversal err = %v, want access denied", err)
	}
}

func TestKnowledgeFilesServiceRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "knowledge")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	svc := NewKnowledgeFilesService(config.KnowledgeConfig{BasePath: base})
	_, err := svc.ReadFile(filepath.Join("link", "secret.md"))
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("ReadFile symlink escape err = %v, want access denied", err)
	}
}
