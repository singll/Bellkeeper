package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type RuleOptimizerService struct {
	cfg        config.CrawlQueueConfig
	ruleRepo   *repository.CrawlExtractionRuleRepository
	jobRepo    *repository.CrawlJobRepository
	llmClient  *llmclient.Client
	extractor  *ExtractorService
	activityLog *ActivityLogService

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewRuleOptimizerService(
	cfg config.CrawlQueueConfig,
	ruleRepo *repository.CrawlExtractionRuleRepository,
	jobRepo *repository.CrawlJobRepository,
	llmBaseURL, apiKey string,
	extractor *ExtractorService,
	activityLog *ActivityLogService,
) *RuleOptimizerService {
	timeout := 3 * time.Minute
	return &RuleOptimizerService{
		cfg:       cfg,
		ruleRepo:  ruleRepo,
		jobRepo:   jobRepo,
		llmClient: llmclient.New(llmclient.Options{BaseURL: llmBaseURL, APIKey: apiKey, Timeout: timeout}),
		extractor: extractor,
		activityLog: activityLog,
	}
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
		if err := s.optimizeDomain(ctx, domain); err != nil {
			log.Printf("[RuleOptimizer] optimize domain %s error: %v", domain, err)
		}
	}
}

func (s *RuleOptimizerService) findOptimizableDomains(ctx context.Context) ([]string, error) {
	since := time.Now().Add(-7 * 24 * time.Hour)
	domains, err := s.jobRepo.FindDeadOrBlockedDomains(since)
	if err != nil {
		return nil, fmt.Errorf("query optimizable domains: %w", err)
	}

	var filtered []string
	for _, d := range domains {
		activeRule, _ := s.ruleRepo.FindActiveByDomain(d)
		if activeRule != nil {
			continue
		}
		candidateRule, _ := s.ruleRepo.FindCandidateByDomain(d)
		if candidateRule != nil {
			trialCount, _ := s.ruleRepo.CountCandidateTrials(candidateRule.ID)
			if int(trialCount) >= s.cfg.RuleMaxTrials {
				continue
			}
		}
		filtered = append(filtered, d)
	}
	return filtered, nil
}

func (s *RuleOptimizerService) optimizeDomain(ctx context.Context, domain string) error {
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

	candidateRule, _ := s.ruleRepo.FindCandidateByDomain(domain)
	if candidateRule == nil {
		rule, err := s.generateRule(ctx, domain, samples)
		if err != nil {
			return fmt.Errorf("generate rule for %s: %w", domain, err)
		}
		if rule == nil {
			log.Printf("[RuleOptimizer] LLM returned no rule for domain %s", domain)
			return nil
		}
		candidateRule = rule
	}

	trial, err := s.validateRule(ctx, candidateRule, samples)
	if err != nil {
		log.Printf("[RuleOptimizer] validate rule %d for %s error: %v", candidateRule.ID, domain, err)
		return nil
	}

	if trial.QualityScore >= 0.6 {
		if err := s.ruleRepo.UpdateStatus(candidateRule.ID, model.ExtractionRuleActive); err != nil {
			return fmt.Errorf("activate rule %d: %w", candidateRule.ID, err)
		}
		log.Printf("[RuleOptimizer] activated rule %d for domain %s (score=%.2f)", candidateRule.ID, domain, trial.QualityScore)
		s.logActivity("rule_activated", "active",
			fmt.Sprintf("Rule %d activated for %s (score=%.2f)", candidateRule.ID, domain, trial.QualityScore))
		s.retryDomainJobs(domain)
	} else {
		trialCount, _ := s.ruleRepo.CountCandidateTrials(candidateRule.ID)
		if int(trialCount) >= s.cfg.RuleMaxTrials {
			if err := s.ruleRepo.UpdateStatus(candidateRule.ID, model.ExtractionRuleRejected); err != nil {
				log.Printf("[RuleOptimizer] reject rule %d error: %v", candidateRule.ID, err)
			}
			log.Printf("[RuleOptimizer] rejected rule %d for %s after %d trials", candidateRule.ID, domain, trialCount)
			s.logActivity("rule_rejected", "rejected",
				fmt.Sprintf("Rule %d rejected for %s after %d trials (score=%.2f)", candidateRule.ID, domain, trialCount, trial.QualityScore))
		} else {
			log.Printf("[RuleOptimizer] trial %d for rule %d/%s scored %.2f (need 0.6), will retry", trialCount, candidateRule.ID, domain, trial.QualityScore)
		}
	}
	return nil
}

type llmRuleOutput struct {
	Strategy           string `json:"strategy"`
	RSSHubRoute        string `json:"rsshub_route,omitempty"`
	CSSTitleSelector   string `json:"css_title_selector,omitempty"`
	CSSContentSelector string `json:"css_content_selector,omitempty"`
	CSSRemoveSelectors string `json:"css_remove_selectors,omitempty"`
	QualityMinChars    int    `json:"quality_min_chars"`
}

func (s *RuleOptimizerService) generateRule(ctx context.Context, domain string, samples []repository.DomainFailureSample) (*model.CrawlExtractionRule, error) {
	samplesJSON, _ := json.Marshal(samples)
	prompt := fmt.Sprintf(`You are a web extraction rule optimizer. Given a domain and its recent extraction failures, suggest an extraction rule.

Domain: %s
Recent failures:
%s

Respond with ONLY a JSON object (no markdown fences):
{
  "strategy": "one of: rsshub | trafilatura | firecrawl | readability",
  "rsshub_route": "RSSHub route path if strategy=rsshub, e.g. /anthropic/news",
  "css_title_selector": "CSS selector for title if strategy=readability",
  "css_content_selector": "CSS selector for article body if strategy=readability",
  "css_remove_selectors": "comma-separated CSS selectors to remove (ads, nav, etc.)",
  "quality_min_chars": 200
}

Rules:
- Prefer "rsshub" if the site has an RSSHub route.
- Use "trafilatura" or "firecrawl" if no RSSHub route exists.
- Use "readability" only if you know specific CSS selectors for the site.
- Do NOT suggest strategies that bypass paywalls or login.
- If you cannot suggest a rule, return {"strategy":"none"}.`, domain, string(samplesJSON))

	resp, err := s.llmClient.ChatCompletion(ctx, llmclient.ChatRequest{
		Model:       "gpt-4o-mini",
		Messages:    []llmclient.ChatMessage{{Role: "user", Content: prompt}},
		Temperature: 0.3,
	}, llmclient.ChatOptions{CallerID: "rule_optimizer", TaskType: "rule_generation"})
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	var output llmRuleOutput
	body := strings.TrimSpace(resp)
	if idx := strings.Index(body, "{"); idx >= 0 {
		last := strings.LastIndex(body, "}")
		if last > idx {
			body = body[idx : last+1]
		}
	}
	if err := json.Unmarshal([]byte(body), &output); err != nil {
		return nil, fmt.Errorf("parse LLM JSON: %w (body=%q)", err, body)
	}
	if output.Strategy == "" || output.Strategy == "none" {
		return nil, nil
	}

	strategy := model.ExtractionStrategy(output.Strategy)
	switch strategy {
	case model.StrategyRSSHub, model.StrategyTrafilatura, model.StrategyFirecrawl, model.StrategyReadability, model.StrategyPlaywright:
	default:
		return nil, fmt.Errorf("unknown strategy: %s", output.Strategy)
	}

	qualityMinChars := output.QualityMinChars
	if qualityMinChars <= 0 {
		qualityMinChars = s.cfg.RuleQualityMinChars
		if qualityMinChars <= 0 {
			qualityMinChars = 200
		}
	}

	rule := &model.CrawlExtractionRule{
		Domain:             domain,
		Strategy:           strategy,
		RSSHubRoute:        output.RSSHubRoute,
		CSSTitleSelector:   output.CSSTitleSelector,
		CSSContentSelector: output.CSSContentSelector,
		CSSRemoveSelectors: output.CSSRemoveSelectors,
		QualityMinChars:    qualityMinChars,
		Version:            1,
		Status:             model.ExtractionRuleCandidate,
		CreatedBy:          model.RuleCreatedByLLM,
	}
	if err := s.ruleRepo.Create(rule); err != nil {
		return nil, fmt.Errorf("create rule: %w", err)
	}
	log.Printf("[RuleOptimizer] created candidate rule %d for domain %s (strategy=%s)", rule.ID, domain, strategy)
	s.logActivity("rule_created", "candidate",
		fmt.Sprintf("Rule %d created for %s (strategy=%s)", rule.ID, domain, strategy))
	return rule, nil
}

func (s *RuleOptimizerService) validateRule(ctx context.Context, rule *model.CrawlExtractionRule, samples []repository.DomainFailureSample) (*model.CrawlRuleTrial, error) {
	sampleURLs := make([]string, 0, len(samples))
	for _, s := range samples {
		sampleURLs = append(sampleURLs, s.URL)
	}
	sampleURLsJSON, _ := json.Marshal(sampleURLs)

	trial := &model.CrawlRuleTrial{
		RuleID:     rule.ID,
		SampleURLs: sampleURLsJSON,
		Attempt:    1,
	}

	existingTrials, _ := s.ruleRepo.ListTrialsByRule(rule.ID)
	trial.Attempt = len(existingTrials) + 1

	if len(samples) > 0 {
		trial.BeforeError = samples[0].ErrorType
	}

	testURL := ""
	for _, s := range samples {
		if s.URL != "" {
			testURL = s.URL
			break
		}
	}

	if testURL == "" {
		trial.AfterStatus = "no_sample_url"
		trial.QualityScore = 0
		_ = s.ruleRepo.CreateTrial(trial)
		return trial, nil
	}

	content, err := s.extractWithRule(ctx, rule, testURL)
	if err != nil {
		trial.AfterStatus = "extraction_failed"
		trial.DiffSummary = truncateStr(err.Error(), 500)
		trial.QualityScore = 0
		_ = s.ruleRepo.CreateTrial(trial)
		return trial, nil
	}

	trial.ContentLen = len(content)
	trial.QualityScore = s.scoreContent(content, rule.QualityMinChars)
	trial.AfterStatus = "extracted"

	if trial.QualityScore >= 0.6 {
		trial.DiffSummary = fmt.Sprintf("Extracted %d chars, score=%.2f", len(content), trial.QualityScore)
	} else {
		trial.DiffSummary = fmt.Sprintf("Low quality: %d chars, score=%.2f", len(content), trial.QualityScore)
	}

	if err := s.ruleRepo.CreateTrial(trial); err != nil {
		log.Printf("[RuleOptimizer] create trial error: %v", err)
	}
	return trial, nil
}

func (s *RuleOptimizerService) extractWithRule(ctx context.Context, rule *model.CrawlExtractionRule, testURL string) (string, error) {
	switch rule.Strategy {
	case model.StrategyRSSHub:
		if rule.RSSHubRoute == "" {
			return "", fmt.Errorf("rsshub route is empty")
		}
		return "", fmt.Errorf("rsshub strategy requires feed reconfiguration, not per-URL extraction")
	case model.StrategyFirecrawl:
		result, err := s.extractor.Extract(&ExtractionRequest{URL: testURL})
		if err != nil {
			return "", err
		}
		if !result.Success {
			return "", fmt.Errorf("%s", result.Error)
		}
		return result.Content, nil
	case model.StrategyTrafilatura:
		result, err := s.extractor.Extract(&ExtractionRequest{URL: testURL})
		if err != nil {
			return "", err
		}
		if !result.Success {
			return "", fmt.Errorf("%s", result.Error)
		}
		return result.Content, nil
	case model.StrategyReadability:
		return "", fmt.Errorf("readability strategy not yet implemented")
	default:
		return "", fmt.Errorf("unsupported strategy: %s", rule.Strategy)
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

func (s *RuleOptimizerService) retryDomainJobs(domain string) {
	if s.jobRepo == nil {
		return
	}
	retried, err := s.jobRepo.RetryDeadOrBlockedByDomain(domain)
	if err != nil {
		log.Printf("[RuleOptimizer] retry domain %s jobs error: %v", domain, err)
	} else if retried > 0 {
		log.Printf("[RuleOptimizer] retried %d jobs for domain %s", retried, domain)
	}
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

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
