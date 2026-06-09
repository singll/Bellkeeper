package handler

import (
	"strconv"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"

	"github.com/gin-gonic/gin"
)

type ExtractionRuleHandler struct {
	ruleSvc *service.RuleOptimizerService
	ruleRepo *repository.CrawlExtractionRuleRepository
}

func NewExtractionRuleHandler(ruleSvc *service.RuleOptimizerService, ruleRepo *repository.CrawlExtractionRuleRepository) *ExtractionRuleHandler {
	return &ExtractionRuleHandler{ruleSvc: ruleSvc, ruleRepo: ruleRepo}
}

func (h *ExtractionRuleHandler) ListRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	domain := c.Query("domain")
	status := c.Query("status")
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}

	opts := repository.ListExtractionRuleOpts{
		Domain: domain,
		Status: status,
		Page:   page,
		Limit:  limit,
	}
	rules, total, err := h.ruleRepo.List(opts)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Page(c, rules, total, page, limit)
}

func (h *ExtractionRuleHandler) GetRule(c *gin.Context) {
	domain := c.Param("domain")
	if domain == "" {
		response.BadRequest(c, "domain parameter required")
		return
	}

	rule, err := h.ruleRepo.FindActiveByDomain(domain)
	if err != nil {
		response.NotFound(c, "rule not found")
		return
	}
	response.Success(c, rule)
}

func (h *ExtractionRuleHandler) CreateRule(c *gin.Context) {
	var req struct {
		Domain             string `json:"domain" binding:"required"`
		MatchPattern       string `json:"match_pattern"`
		Strategy           string `json:"strategy" binding:"required"`
		RSSHubRoute        string `json:"rsshub_route"`
		CSSTitleSelector   string `json:"css_title_selector"`
		CSSContentSelector string `json:"css_content_selector"`
		CSSRemoveSelectors string `json:"css_remove_selectors"`
		QualityMinChars    int    `json:"quality_min_chars"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	strategy := model.ExtractionStrategy(req.Strategy)
	switch strategy {
	case model.StrategyRSSHub, model.StrategyTrafilatura, model.StrategyFirecrawl, model.StrategyReadability, model.StrategyPlaywright:
	default:
		response.BadRequest(c, "invalid strategy")
		return
	}

	qualityMinChars := req.QualityMinChars
	if qualityMinChars <= 0 {
		qualityMinChars = 200
	}

	rule := &model.CrawlExtractionRule{
		Domain:             req.Domain,
		MatchPattern:       req.MatchPattern,
		Strategy:           strategy,
		RSSHubRoute:        req.RSSHubRoute,
		CSSTitleSelector:   req.CSSTitleSelector,
		CSSContentSelector: req.CSSContentSelector,
		CSSRemoveSelectors: req.CSSRemoveSelectors,
		QualityMinChars:    qualityMinChars,
		Version:            1,
		Status:             model.ExtractionRuleCandidate,
		CreatedBy:          model.RuleCreatedByHuman,
	}
	if err := h.ruleRepo.Create(rule); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, rule)
}

func (h *ExtractionRuleHandler) UpdateRuleStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	status := model.ExtractionRuleStatus(req.Status)
	switch status {
	case model.ExtractionRuleCandidate, model.ExtractionRuleActive, model.ExtractionRuleRejected, model.ExtractionRuleRollback:
	default:
		response.BadRequest(c, "invalid status")
		return
	}

	if err := h.ruleRepo.UpdateStatus(uint(id), status); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "status": req.Status})
}

func (h *ExtractionRuleHandler) ListTrials(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	trials, err := h.ruleRepo.ListTrialsByRule(uint(id))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, trials)
}
