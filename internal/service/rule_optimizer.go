package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/llmgateway"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"gorm.io/datatypes"
)

type RuleOptimizerService struct {
	cfg         config.CrawlQueueConfig
	ruleRepo    *repository.CrawlExtractionRuleRepository
	domainRepo  *repository.CrawlDomainProfileRepository
	failureRepo *repository.CrawlFailureRepository
	llm         llmgateway.Gateway
	extractor   *ExtractorService
	activityLog *ActivityLogService
	promptLoader *KBPromptLoader

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewRuleOptimizerService(
	cfg config.CrawlQueueConfig,
	ruleRepo *repository.CrawlExtractionRuleRepository,
	domainRepo *repository.CrawlDomainProfileRepository,
	failureRepo *repository.CrawlFailureRepository,
	llm llmgateway.Gateway,
	extractor *ExtractorService,
	activityLog *ActivityLogService,
) *RuleOptimizerService {
	return &RuleOptimizerService{
		cfg:         cfg,
		ruleRepo:    ruleRepo,
		domainRepo:  domainRepo,
		failureRepo: failureRepo,
		llm:         llm,
		extractor:   extractor,
		activityLog: activityLog,
	}
}

// SetPromptLoader 注入知识库提示词加载器（1.0 §2.1.3）。
func (s *RuleOptimizerService) SetPromptLoader(loader *KBPromptLoader) {
	s.promptLoader = loader
}

func (s *RuleOptimizerService) Start(ctx context.Context) {
	if !s.cfg.RuleOptimizerEnabled || s.ruleRepo == nil {
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	interval := time.Duration(s.cfg.RuleOptimizerInterval) * time.Minute
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	s.wg.Add(1)
	go s.loop(ctx, interval)
	log.Printf("[RuleOptimizer] started: interval=%s", interval)
}

func (s *RuleOptimizerService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	log.Printf("[RuleOptimizer] stopped")
}

func (s *RuleOptimizerService) loop(ctx context.Context, interval time.Duration) {
	defer s.wg.Done()
	s.runOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *RuleOptimizerService) runOnce(ctx context.Context) {
	domains, err := s.findOptimizableDomains(ctx)
	if err != nil {
		log.Printf("[RuleOptimizer] find domains error: %v", err)
		return
	}
	if len(domains) == 0 {
		return
	}
	log.Printf("[RuleOptimizer] found %d optimizable domains", len(domains))
	for _, domain := range domains {
		if ctx.Err() != nil {
			return
		}
		if err := s.AnalyzeDomain(ctx, domain); err != nil {
			log.Printf("[RuleOptimizer] optimize domain %s error: %v", domain, err)
		}
	}
}

func (s *RuleOptimizerService) findOptimizableDomains(ctx context.Context) ([]string, error) {
	if s.domainRepo == nil {
		return nil, nil
	}
	domains, err := s.domainRepo.FindCoolingWithoutOverrides(50)
	if err != nil {
		return nil, fmt.Errorf("query optimizable domains: %w", err)
	}

	var filtered []string
	for _, d := range domains {
		if s.isPaywallDomain(d) {
			continue
		}
		if s.hasRecentAnalysis(d) {
			continue
		}
		filtered = append(filtered, d)
	}
	return filtered, nil
}

func (s *RuleOptimizerService) isPaywallDomain(domain string) bool {
	if s.failureRepo == nil {
		return false
	}
	failure, err := s.failureRepo.FindByDomain(domain)
	if err != nil || failure == nil {
		return false
	}
	return failure.LastErrorType == "paywall"
}

func (s *RuleOptimizerService) hasRecentAnalysis(domain string) bool {
	if s.domainRepo == nil {
		return false
	}
	profile, err := s.domainRepo.FindByDomain(domain)
	if err != nil || profile == nil {
		return false
	}
	return profile.AnalysisResult != "" && time.Since(profile.UpdatedAt) < 24*time.Hour
}

// browserUserAgent is the spoofed UA applied by the deterministic 403 rule.
const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// defaultOverridesFor returns deterministic request overrides for well-understood
// failure types, covering the common cases without an LLM call. Returns (nil, "")
// when the error type isn't deterministically fixable, signalling the caller to
// fall back to LLM analysis.
func defaultOverridesFor(errType string) (*RequestOverrides, string) {
	switch errType {
	case "forbidden": // HTTP 403 — most often a missing/blocked UA
		return &RequestOverrides{
			UserAgent: browserUserAgent,
			Headers: map[string]string{
				"Accept-Language": "en-US,en;q=0.9",
				"Referer":         "https://www.google.com/",
			},
		}, "403 Forbidden → spoof a real browser User-Agent plus Referer/Accept-Language headers"
	case "timeout", "network": // slow or JS-heavy site
		return &RequestOverrides{Strategy: "firecrawl", TimeoutSeconds: 120}, "timeout/network error → switch to firecrawl with a longer timeout"
	case "empty_content": // content rendered client-side
		return &RequestOverrides{Strategy: "firecrawl", FirecrawlWaitFor: 3000}, "empty content → likely JS-rendered; use firecrawl and wait for the DOM"
	case "rate_limited", "server_error": // transient — handled by job/domain backoff, not overrides
		return nil, ""
	default:
		return nil, "" // unknown → let the LLM analyze
	}
}

// dominantErrorType returns the most frequent error type across the samples.
func dominantErrorType(samples []repository.DomainFailureSample) string {
	counts := make(map[string]int)
	for _, sample := range samples {
		if sample.ErrorType != "" {
			counts[sample.ErrorType]++
		}
	}
	best, bestN := "", 0
	for errType, n := range counts {
		if n > bestN {
			best, bestN = errType, n
		}
	}
	return best
}

func (s *RuleOptimizerService) AnalyzeDomain(ctx context.Context, domain string) error {
	if s.isPaywallDomain(domain) {
		log.Printf("[RuleOptimizer] skipping paywall domain %s (省Token)", domain)
		return nil
	}

	sampleSize := s.cfg.RuleSampleSize
	if sampleSize <= 0 {
		sampleSize = 5
	}
	since := time.Now().Add(-7 * 24 * time.Hour)
	samples, err := s.ruleRepo.CollectFailureSamples(domain, sampleSize, since)
	if err != nil {
		return fmt.Errorf("collect samples for %s: %w", domain, err)
	}
	if len(samples) == 0 {
		return nil
	}

	// 1) Deterministic rules table first — covers most failures without an LLM call.
	errType := dominantErrorType(samples)
	if overrides, analysis := defaultOverridesFor(errType); overrides != nil {
		if err := s.persistOverrides(domain, overrides, analysis); err != nil {
			return err
		}
		log.Printf("[RuleOptimizer] rule-based overrides for %s (errType=%s)", domain, errType)
		s.logActivity("overrides_rule", "cooling",
			fmt.Sprintf("Rule-based overrides for %s (errType=%s)", domain, errType))
		s.validateOverrides(ctx, domain, overrides, samples)
		return nil
	}

	// 2) LLM fallback only when the rules table doesn't cover this error type.
	overrides, analysis, err := s.generateRequestOverrides(ctx, domain, samples)
	if err != nil {
		return fmt.Errorf("generate overrides for %s: %w", domain, err)
	}
	if overrides == nil {
		// Record the analysis (e.g. "paywall / unfixable") so we don't re-analyze
		// this domain every cycle, even though there are no overrides to apply.
		if analysis != "" {
			if err := s.persistAnalysisOnly(domain, analysis); err != nil {
				log.Printf("[RuleOptimizer] persist analysis-only for %s failed: %v", domain, err)
			}
		}
		log.Printf("[RuleOptimizer] LLM returned no overrides for domain %s", domain)
		return nil
	}

	if err := s.persistOverrides(domain, overrides, analysis); err != nil {
		return err
	}
	log.Printf("[RuleOptimizer] LLM-generated overrides for domain %s", domain)
	s.logActivity("overrides_generated", "cooling",
		fmt.Sprintf("LLM request overrides generated for %s", domain))

	s.validateOverrides(ctx, domain, overrides, samples)

	return nil
}

// persistOverrides stores domain-level overrides + analysis on the profile.
func (s *RuleOptimizerService) persistOverrides(domain string, overrides *RequestOverrides, analysis string) error {
	if s.domainRepo == nil {
		return nil
	}
	overridesJSON, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("marshal overrides: %w", err)
	}
	if err := s.domainRepo.UpdateOverrides(domain, datatypes.JSON(overridesJSON), analysis); err != nil {
		return fmt.Errorf("update overrides for %s: %w", domain, err)
	}
	return nil
}

// persistAnalysisOnly records analysis text without overrides (e.g. unfixable domains).
func (s *RuleOptimizerService) persistAnalysisOnly(domain, analysis string) error {
	if s.domainRepo == nil {
		return nil
	}
	return s.domainRepo.UpdateOverrides(domain, datatypes.JSON("null"), analysis)
}

type llmOverridesOutput struct {
	UserAgent        string            `json:"user_agent,omitempty"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Strategy         string            `json:"strategy,omitempty"`
	FirecrawlWaitFor int               `json:"firecrawl_wait_for,omitempty"`
	FirecrawlActions []FirecrawlAction `json:"firecrawl_actions,omitempty"`
	Analysis         string            `json:"analysis,omitempty"`
	None             bool              `json:"none,omitempty"`
}

func (s *RuleOptimizerService) generateRequestOverrides(ctx context.Context, domain string, samples []repository.DomainFailureSample) (*RequestOverrides, string, error) {
	samplesJSON, _ := json.Marshal(samples)
	// 1.0 §2.1.3：system/user 分离 + 外置提示词（loader 缺失回退内置）+ ResponseFormat=json_object。
	systemPrompt := s.promptLoader.GetWithDefault("rule_optimizer_system", "You are a web extraction failure analyst.")
	userTemplate := s.promptLoader.GetWithDefault("rule_optimizer_user", "Domain: %s\nRecent failures:\n%s\n")
	userContent := fmt.Sprintf(userTemplate, domain, string(samplesJSON))

	llmModel := s.cfg.RuleOptimizerModel
	if llmModel == "" {
		llmModel = "pool-summary"
	}
	temp := s.cfg.RuleOptimizerTemperature
	if temp <= 0 {
		temp = 0.3
	}
	resp, err := s.llm.Chat(ctx, llmclient.ChatRequest{
		Model:          llmModel,
		Messages:       []llmclient.ChatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userContent}},
		Temperature:    temp,
		ResponseFormat: &llmclient.ResponseFormat{Type: "json_object"},
	}, llmclient.ChatOptions{CallerID: "rule_optimizer", TaskType: string(llmgateway.TaskRuleGeneration)})
	if err != nil {
		return nil, "", fmt.Errorf("LLM call failed: %w", err)
	}

	var output llmOverridesOutput
	body := strings.TrimSpace(resp.Content)
	if idx := strings.Index(body, "{"); idx >= 0 {
		last := strings.LastIndex(body, "}")
		if last > idx {
			body = body[idx : last+1]
		}
	}
	if err := json.Unmarshal([]byte(body), &output); err != nil {
		return nil, "", fmt.Errorf("parse LLM JSON: %w (body=%q)", err, body)
	}
	if output.None {
		return nil, output.Analysis, nil
	}

	overrides := &RequestOverrides{
		UserAgent:        output.UserAgent,
		TimeoutSeconds:   output.TimeoutSeconds,
		Headers:          output.Headers,
		Strategy:         output.Strategy,
		FirecrawlWaitFor: output.FirecrawlWaitFor,
		FirecrawlActions: output.FirecrawlActions,
	}
	// 净化：Firecrawl 实例不支持 actions 时，丢弃 LLM 幻觉出的 actions，避免把注定
	// HTTP 400（SCRAPE_ACTIONS_NOT_SUPPORTED）的参数写进 domain profile 反复刷失败。
	// 与 extractor 下发处形成纵深防御：既不下发，也不持久化。
	if len(overrides.FirecrawlActions) > 0 && s.extractor != nil && !s.extractor.FirecrawlSupportsActions() {
		log.Printf("[RuleOptimizer] dropping %d LLM-suggested firecrawl actions for %s (instance has no Fire Engine)",
			len(overrides.FirecrawlActions), domain)
		overrides.FirecrawlActions = nil
	}
	return overrides, output.Analysis, nil
}

func (s *RuleOptimizerService) validateOverrides(ctx context.Context, domain string, overrides *RequestOverrides, samples []repository.DomainFailureSample) {
	testURL := ""
	for _, sample := range samples {
		if sample.URL != "" {
			testURL = sample.URL
			break
		}
	}
	if testURL == "" {
		return
	}

	result, err := s.extractor.Extract(&ExtractionRequest{
		URL:      testURL,
		Overrides: overrides,
	})
	if err != nil {
		log.Printf("[RuleOptimizer] validation failed for %s: %v", domain, err)
		s.logActivity("validation_failed", "cooling",
			fmt.Sprintf("Override validation failed for %s: %v", domain, err))
		return
	}
	if !result.Success {
		log.Printf("[RuleOptimizer] validation extracted but failed for %s: %s", domain, result.Error)
		s.logActivity("validation_failed", "cooling",
			fmt.Sprintf("Override validation failed for %s: %s", domain, result.Error))
		return
	}

	minChars := s.cfg.RuleQualityMinChars
	if minChars <= 0 {
		minChars = 200
	}
	score := s.scoreContent(result.Content, minChars)
	log.Printf("[RuleOptimizer] validation for %s: extractor=%s len=%d score=%.2f",
		domain, result.Extractor, len(result.Content), score)

	if score >= 0.6 {
		s.logActivity("validation_passed", "cooling",
			fmt.Sprintf("Override validated for %s: score=%.2f extractor=%s len=%d",
				domain, score, result.Extractor, len(result.Content)))
	} else {
		s.logActivity("validation_low_quality", "cooling",
			fmt.Sprintf("Override low quality for %s: score=%.2f", domain, score))
	}
}

func (s *RuleOptimizerService) scoreContent(content string, minChars int) float64 {
	if len(content) == 0 {
		return 0
	}
	score := 0.0

	if len(content) >= minChars {
		score += 0.4
	} else if len(content) >= minChars/2 {
		score += 0.2
	}

	lower := strings.ToLower(content)
	paywallHits := 0
	for _, kw := range []string{
		"subscribe to continue", "sign in to read", "premium content",
		"subscription required", "members only", "paid content",
		"订阅后查看", "登录后阅读", "付费内容", "会员专享",
	} {
		if strings.Contains(lower, kw) {
			paywallHits++
		}
	}
	if paywallHits == 0 {
		score += 0.3
	} else {
		score -= float64(paywallHits) * 0.2
	}

	words := len(strings.Fields(content))
	if words > 50 {
		score += 0.2
	} else if words > 20 {
		score += 0.1
	}

	noisy := 0
	for _, kw := range []string{"cookie", "javascript is required", "enable javascript", "ad-blocker"} {
		if strings.Contains(lower, kw) {
			noisy++
		}
	}
	score -= float64(noisy) * 0.1

	if score > 1.0 {
		score = 1.0
	}
	if score < 0 {
		score = 0
	}
	return score
}

// --- Extraction Rule CRUD (1.0 §5 分层合规：handler 不持 repo，全部经 service) ---

// ListExtractionRules 列出抽取规则。
func (s *RuleOptimizerService) ListExtractionRules(opts repository.ListExtractionRuleOpts) ([]model.CrawlExtractionRule, int64, error) {
	return s.ruleRepo.List(opts)
}

// GetExtractionRule 获取域名抽取规则。
func (s *RuleOptimizerService) GetExtractionRule(domain string) (*model.CrawlExtractionRule, error) {
	return s.ruleRepo.FindActiveByDomain(domain)
}

// CreateExtractionRule 创建抽取规则。
func (s *RuleOptimizerService) CreateExtractionRule(rule *model.CrawlExtractionRule) error {
	return s.ruleRepo.Create(rule)
}

// UpdateExtractionRuleStatus 更新规则状态。
func (s *RuleOptimizerService) UpdateExtractionRuleStatus(id uint, status model.ExtractionRuleStatus) error {
	return s.ruleRepo.UpdateStatus(id, status)
}

// ListExtractionTrials 列出规则试跑记录。
func (s *RuleOptimizerService) ListExtractionTrials(ruleID uint) ([]model.CrawlRuleTrial, error) {
	return s.ruleRepo.ListTrialsByRule(ruleID)
}

func (s *RuleOptimizerService) logActivity(action, status, summary string) {
	if s.activityLog == nil {
		return
	}
	s.activityLog.LogActivity(LogActivityParams{
		Module:  "rule_optimizer",
		Action:  action,
		Status:  status,
		Summary: summary,
	})
}
