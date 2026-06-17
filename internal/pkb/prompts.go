package pkb

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PromptRegistry pins the active prompt files so prompt versions can be changed
// without editing Go code or overwriting older prompt revisions.
type PromptRegistry struct {
	Active PromptRegistryActive `yaml:"active"`
}

type PromptRegistryActive struct {
	Score           string `yaml:"score"`
	Reconstruct     string `yaml:"reconstruct"`
	Digest          string `yaml:"digest"`
	DigestTopic     string `yaml:"digest_topic"`
	Skeleton        string `yaml:"skeleton"`
	Match           string `yaml:"match"`
	SkeletonPropose string `yaml:"skeleton_propose"`
}

func LoadPromptRegistry(configDir string) (*PromptRegistry, error) {
	reg := &PromptRegistry{}
	reg.Active.Score = "score.md"
	reg.Active.Reconstruct = "reconstruct.md"
	reg.Active.Digest = "digest.md"

	path := filepath.Join(configDir, "prompts", "registry.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, fmt.Errorf("read prompt registry: %w", err)
	}
	if err := yaml.Unmarshal(data, reg); err != nil {
		return nil, fmt.Errorf("parse prompt registry: %w", err)
	}
	if reg.Active.Score == "" {
		reg.Active.Score = "score.md"
	}
	if reg.Active.Reconstruct == "" {
		reg.Active.Reconstruct = "reconstruct.md"
	}
	if reg.Active.Digest == "" {
		reg.Active.Digest = "digest.md"
	}
	if reg.Active.DigestTopic == "" {
		reg.Active.DigestTopic = "digest.topic.md"
	}
	if reg.Active.Skeleton == "" {
		reg.Active.Skeleton = "skeleton.md"
	}
	if reg.Active.Match == "" {
		reg.Active.Match = "match.md"
	}
	if reg.Active.SkeletonPropose == "" {
		reg.Active.SkeletonPropose = "skeleton_propose.md"
	}
	return reg, nil
}

func loadPromptFile(configDir, name string) (string, error) {
	clean := filepath.Clean(name)
	if filepath.IsAbs(name) || clean != name || name == "." || name == ".." || filepath.Base(name) != name {
		return "", fmt.Errorf("invalid prompt file name: %s", name)
	}
	data, err := os.ReadFile(filepath.Join(configDir, "prompts", name))
	if err != nil {
		return "", fmt.Errorf("read prompt %s: %w", name, err)
	}
	return string(data), nil
}
