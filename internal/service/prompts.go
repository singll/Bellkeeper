// prompts.go 实现知识库非 PKB 专属提示词的统一加载（classify / knowledge_ask /
// rule_optimizer），复用 PKB 的 registry + 文件外置模式。
//
// 对应《Bellkeeper 1.0 重构与架构演进规划》§2.1.3：
//   - 提示词外置到 config/prompts/ + registry.yaml（与 config/pkb/prompts/ 并列）
//   - system/user 角色分离（规则段入 system，数据段入 user），利用 provider prompt cache
//   - 配合 ChatRequest.ResponseFormat/MaxTokens 做结构化输出
//
// 加载失败时回退到内置默认提示词（保证服务可启动），并记录 warning。

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// PromptKey 标识一组提示词（system + user 模板）。
type PromptKey string

const (
	PromptKeyClassify        PromptKey = "classify"
	PromptKeyKnowledgeAsk    PromptKey = "knowledge_ask"
	PromptKeyRuleOptimizer   PromptKey = "rule_optimizer"
)

// kbPromptRegistry 是 config/prompts/registry.yaml 的结构。
type kbPromptRegistry struct {
	Active kbPromptRegistryActive `yaml:"active"`
}

type kbPromptRegistryActive struct {
	ClassifySystem       string `yaml:"classify_system"`
	ClassifyUser         string `yaml:"classify_user"`
	KnowledgeAskSystem   string `yaml:"knowledge_ask_system"`
	RuleOptimizerSystem  string `yaml:"rule_optimizer_system"`
	RuleOptimizerUser    string `yaml:"rule_optimizer_user"`
}

// KBPromptLoader 加载并缓存知识库提示词（线程安全）。
type KBPromptLoader struct {
	configDir string
	once      sync.Once
	registry  kbPromptRegistry
	cache     map[string]string
	loadErr   error
}

// NewKBPromptLoader 构造加载器。configDir 形如 "config"（含 prompts/ 子目录）。
func NewKBPromptLoader(configDir string) *KBPromptLoader {
	return &KBPromptLoader{configDir: configDir, cache: make(map[string]string)}
}

// load 一次性加载 registry + 全部提示词文件到缓存。
func (l *KBPromptLoader) load() {
	l.once.Do(func() {
		// 默认值（registry 缺失时回退）
		l.registry = kbPromptRegistry{
			Active: kbPromptRegistryActive{
				ClassifySystem:      "classify_system.md",
				ClassifyUser:        "classify_user.md",
				KnowledgeAskSystem:  "knowledge_ask_system.md",
				RuleOptimizerSystem: "rule_optimizer_system.md",
				RuleOptimizerUser:   "rule_optimizer_user.md",
			},
		}
		regPath := filepath.Join(l.configDir, "prompts", "registry.yaml")
		if data, err := os.ReadFile(regPath); err == nil {
			if err := yaml.Unmarshal(data, &l.registry); err != nil {
				l.loadErr = fmt.Errorf("parse kb prompt registry: %w", err)
				return
			}
		}
		// 加载全部提示词文件到缓存
		files := map[string]string{
			"classify_system":        l.registry.Active.ClassifySystem,
			"classify_user":          l.registry.Active.ClassifyUser,
			"knowledge_ask_system":   l.registry.Active.KnowledgeAskSystem,
			"rule_optimizer_system":  l.registry.Active.RuleOptimizerSystem,
			"rule_optimizer_user":    l.registry.Active.RuleOptimizerUser,
		}
		for key, name := range files {
			if name == "" {
				continue
			}
			content, err := l.readFile(name)
			if err != nil {
				l.loadErr = fmt.Errorf("load prompt %s (%s): %w", key, name, err)
				return
			}
			l.cache[key] = content
		}
	})
}

// readFile 读取提示词文件（路径安全：禁止绝对路径/遍历）。
func (l *KBPromptLoader) readFile(name string) (string, error) {
	clean := filepath.Clean(name)
	if filepath.IsAbs(name) || clean != name || name == "." || name == ".." || filepath.Base(name) != name {
		return "", fmt.Errorf("invalid prompt file name: %s", name)
	}
	data, err := os.ReadFile(filepath.Join(l.configDir, "prompts", name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Get 获取指定 key 的提示词模板。加载失败或缺失时返回 ("", error)。
func (l *KBPromptLoader) Get(key string) (string, error) {
	l.load()
	if l.loadErr != nil {
		return "", l.loadErr
	}
	s, ok := l.cache[key]
	if !ok {
		return "", fmt.Errorf("prompt not found: %s", key)
	}
	return s, nil
}

// GetWithDefault 获取提示词，缺失时回退到 fallback（保证服务可启动）。
func (l *KBPromptLoader) GetWithDefault(key, fallback string) string {
	s, err := l.Get(key)
	if err != nil || s == "" {
		return fallback
	}
	return s
}