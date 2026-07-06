// admin.go 实现 LLM 代理池的管理面 service（LLMAdminService），消化分层例外②：
// handler 不再直接持有 repository，而是经 AdminService 访问 token / pricing /
// usage 管理 CRUD 与计费试算。
//
// 同时定义 TokenScopeService 接口（消化分层例外①）：供 auth.LLMTokenAuth
// 中间件依赖的 token 鉴权 + 配额查询能力，router 不再传具体 repository。
//
// 见《Bellkeeper 1.0 重构与架构演进规划》§2.2.2 与 §4 P1 [llmgw] 消化分层例外。

package llmgateway

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// TokenScopeService 是 token 鉴权与配额查询的 service 层抽象（消化例外①）。
//
// auth.LLMTokenAuth 中间件依赖此接口而非 *repository.LLMTokenRepository，
// 使 router 层不再持有/传递具体 repository。*LLMAdminService 实现该接口
// （底层委托 tokenRepo），亦兼容既有 *repository.LLMTokenRepository 隐式实现。
type TokenScopeService interface {
	GetByKeyHash(hash string) (*model.LLMToken, error)
	IsModelGroupName(name string) (bool, error)
	CountRequestsToday(tokenID uint) (int, error)
	TokensUsedToday(tokenID uint) (int, error)
	CostThisMonthCents(tokenID uint) (int, error)
	UpdateLastUsed(tokenID uint) error
}

// Compile-time: *LLMAdminService implements TokenScopeService.
var _ TokenScopeService = (*LLMAdminService)(nil)

// LLMAdminService 收拢 LLM 代理池管理面（token / pricing / usage CRUD + 计费试算），
// 消化分层例外②——LLMProxyHandler 不再持有 repository，仅依赖本 service。
type LLMAdminService struct {
	tokenRepo      *repository.LLMTokenRepository
	tokenUsageRepo *repository.LLMTokenUsageRepository
	pricingRepo    *repository.LLMModelPricingRepository
	pricer         *Pricer
}

// NewLLMAdminService 构造管理面 service。pricer 可为 nil（仅当不需计费试算时）。
func NewLLMAdminService(
	tokenRepo *repository.LLMTokenRepository,
	tokenUsageRepo *repository.LLMTokenUsageRepository,
	pricingRepo *repository.LLMModelPricingRepository,
	pricer *Pricer,
) *LLMAdminService {
	return &LLMAdminService{
		tokenRepo:      tokenRepo,
		tokenUsageRepo: tokenUsageRepo,
		pricingRepo:    pricingRepo,
		pricer:         pricer,
	}
}

// ===================== TokenScopeService 实现（例外①） =====================

func (a *LLMAdminService) GetByKeyHash(hash string) (*model.LLMToken, error) {
	return a.tokenRepo.GetByKeyHash(hash)
}

func (a *LLMAdminService) IsModelGroupName(name string) (bool, error) {
	return a.tokenRepo.IsModelGroupName(name)
}

func (a *LLMAdminService) CountRequestsToday(tokenID uint) (int, error) {
	return a.tokenRepo.CountRequestsToday(tokenID)
}

func (a *LLMAdminService) TokensUsedToday(tokenID uint) (int, error) {
	return a.tokenRepo.TokensUsedToday(tokenID)
}

func (a *LLMAdminService) CostThisMonthCents(tokenID uint) (int, error) {
	return a.tokenRepo.CostThisMonthCents(tokenID)
}

func (a *LLMAdminService) UpdateLastUsed(tokenID uint) error {
	return a.tokenRepo.UpdateLastUsed(tokenID)
}

// ===================== Token 管理 CRUD（例外②） =====================

// ListTokens 返回全部 token（KeyHash 清空，不外泄）。
func (a *LLMAdminService) ListTokens() ([]model.LLMToken, error) {
	tokens, err := a.tokenRepo.List()
	if err != nil {
		return nil, err
	}
	for i := range tokens {
		tokens[i].KeyHash = ""
	}
	return tokens, nil
}

// CreateTokenRequest 是创建 token 的入参。
type CreateTokenRequest struct {
	Name                  string
	CallerID              string
	AllowedModels         []string
	AllowedGroups         []string
	QuotaRequestsDaily    int
	QuotaTokensDaily      int
	QuotaCostMonthlyCents int
	ExpiresAt             *time.Time
}

// CreateTokenResult 是创建 token 的结果（含仅此次返回的明文 key）。
type CreateTokenResult struct {
	Token model.LLMToken
	Key   string
}

// CreateToken 创建 token 并返回明文 key（仅此次返回）。
func (a *LLMAdminService) CreateToken(req CreateTokenRequest) (*CreateTokenResult, error) {
	if req.Name == "" || req.CallerID == "" {
		return nil, fmt.Errorf("name and caller_id are required")
	}
	rawKey, err := generateTokenKey()
	if err != nil {
		return nil, err
	}
	token := model.LLMToken{
		Name:                  req.Name,
		KeyHash:               model.HashKey(rawKey),
		KeyPrefix:             rawKey[:min(8, len(rawKey))],
		CallerID:              req.CallerID,
		QuotaRequestsDaily:    req.QuotaRequestsDaily,
		QuotaTokensDaily:      req.QuotaTokensDaily,
		QuotaCostMonthlyCents: req.QuotaCostMonthlyCents,
		ExpiresAt:             req.ExpiresAt,
	}
	token.SetAllowedModels(req.AllowedModels)
	token.SetAllowedGroups(req.AllowedGroups)
	if err := a.tokenRepo.Create(&token); err != nil {
		return nil, err
	}
	return &CreateTokenResult{Token: token, Key: rawKey}, nil
}

// UpdateTokenRequest 是更新 token 的入参（零值表示不更新对应字段，指针类除外）。
type UpdateTokenRequest struct {
	Name                  string
	AllowedModels         []string
	AllowedGroups         []string
	QuotaRequestsDaily    int
	QuotaTokensDaily      int
	QuotaCostMonthlyCents int
	Enabled               *bool
	ExpiresAt             *time.Time
}

// UpdateToken 更新指定 token，返回更新后的 token（KeyHash 清空）。
func (a *LLMAdminService) UpdateToken(id uint, req UpdateTokenRequest) (*model.LLMToken, error) {
	token, err := a.tokenRepo.Get(id)
	if err != nil {
		return nil, fmt.Errorf("token not found: %w", err)
	}
	if req.Name != "" {
		token.Name = req.Name
	}
	token.SetAllowedModels(req.AllowedModels)
	token.SetAllowedGroups(req.AllowedGroups)
	token.QuotaRequestsDaily = req.QuotaRequestsDaily
	token.QuotaTokensDaily = req.QuotaTokensDaily
	token.QuotaCostMonthlyCents = req.QuotaCostMonthlyCents
	if req.Enabled != nil {
		token.Enabled = *req.Enabled
	}
	if req.ExpiresAt != nil {
		token.ExpiresAt = req.ExpiresAt
	}
	if err := a.tokenRepo.Update(token); err != nil {
		return nil, err
	}
	token.KeyHash = ""
	return token, nil
}

// DeleteToken 删除指定 token。
func (a *LLMAdminService) DeleteToken(id uint) error {
	return a.tokenRepo.Delete(id)
}

// RegenerateTokenKey 为指定 token 重生成明文 key，返回新明文 key（仅此次返回）。
func (a *LLMAdminService) RegenerateTokenKey(id uint) (string, error) {
	token, err := a.tokenRepo.Get(id)
	if err != nil {
		return "", fmt.Errorf("token not found: %w", err)
	}
	rawKey, err := generateTokenKey()
	if err != nil {
		return "", err
	}
	token.KeyHash = model.HashKey(rawKey)
	token.KeyPrefix = rawKey[:min(8, len(rawKey))]
	if err := a.tokenRepo.Update(token); err != nil {
		return "", err
	}
	return rawKey, nil
}

// GetTokenUsage 返回指定 token 在最近 days 天的用量记录。
func (a *LLMAdminService) GetTokenUsage(id uint, days int) ([]model.LLMTokenUsageDaily, error) {
	if days <= 0 {
		days = 7
	}
	if days > 90 {
		days = 90
	}
	to := time.Now()
	from := to.AddDate(0, 0, -days)
	return a.tokenUsageRepo.ListByToken(id, from, to)
}

// ===================== Pricing 管理 CRUD（例外②） =====================

// ListPricing 返回全部定价配置。
func (a *LLMAdminService) ListPricing() ([]model.LLMModelPricing, error) {
	return a.pricingRepo.List()
}

// CreatePricing 创建定价配置。
func (a *LLMAdminService) CreatePricing(p *model.LLMModelPricing) error {
	if p.ChannelName == "" || p.Model == "" {
		return fmt.Errorf("channel_name and model are required")
	}
	return a.pricingRepo.Create(p)
}

// UpdatePricing 更新定价配置。
func (a *LLMAdminService) UpdatePricing(p *model.LLMModelPricing) error {
	return a.pricingRepo.Update(p)
}

// DeletePricing 删除定价配置。
func (a *LLMAdminService) DeletePricing(id uint) error {
	return a.pricingRepo.Delete(id)
}

// CalcCost 按当前定价试算一次用量的成本（分）。
func (a *LLMAdminService) CalcCost(channelName, model string, usage Usage) (int, error) {
	if a.pricer == nil {
		return 0, fmt.Errorf("pricer not configured")
	}
	return a.pricer.Calc(channelName, model, usage)
}

// ===================== 内部工具 =====================

func generateTokenKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate key: %w", err)
	}
	return "sk-bk-" + hex.EncodeToString(b), nil
}
