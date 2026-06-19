package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkb"
	"github.com/singll/bellkeeper/internal/pkg/response"
)

// SkeletonRunner 注入的骨架生成触发器（app 层在 scheduler 就绪后提供，避免 handler 依赖构造顺序）。
type SkeletonRunner func(domain string) error

// SkeletonStatusFn 注入的骨架排队/生成中状态查询（返回排队域名列表 + 当前正在生成的域名）。
type SkeletonStatusFn func() (queued []string, running string)

// PKBSteerHandler 知识骨架「调方向」掌舵面（Phase I / ADR-0004 Q9/Q12）：把原本只能走
// Matrix !pkb / CLI 的「待批骨架提议审批」与「领域大方向(scope)设定」搬到前端 REST，
// 让人用窄掌舵面调方向。浏览仍归 Obsidian、骨架文件仍机器独占写（W1），本 handler 不做文件编辑。
type PKBSteerHandler struct {
	basePath       string           // vault 根（待批提议落 basePath/_提议/<id>.md）
	domainsPath    string           // config/pkb/domains.yaml（设 scope 的落点）
	skeletonRunner SkeletonRunner   // 后台触发骨架生成（由 app 注入 scheduler.TriggerSkeleton）
	skeletonStatus SkeletonStatusFn // 查排队/生成中状态（app 注入 scheduler.SkeletonStatus）
	waitlistHigh   int              // 总览「需要关注」待归位阈值（≤0 关闭标记，app 据 config 注入）
	lowConfHigh    int              // 总览「需要关注」低置信阈值（≤0 关闭标记）
}

// SetSkeletonRunner 注入骨架生成触发器（app 在 pkbScheduler 构造后调用）。
func (h *PKBSteerHandler) SetSkeletonRunner(fn SkeletonRunner) { h.skeletonRunner = fn }

// SetSkeletonStatusFn 注入骨架排队/生成中状态查询（app 在 pkbScheduler 构造后调用）。
func (h *PKBSteerHandler) SetSkeletonStatusFn(fn SkeletonStatusFn) { h.skeletonStatus = fn }

// SetOverviewThresholds 注入总览「需要关注」阈值（app 据 config.Knowledge 注入；≤0 关闭对应标记）。
func (h *PKBSteerHandler) SetOverviewThresholds(waitlist, lowConf int) {
	h.waitlistHigh = waitlist
	h.lowConfHigh = lowConf
}

// NewPKBSteerHandler 构造掌舵面 handler。basePath 为 vault 根（同 Matrix !pkb 闭包取值），
// domainsPath 为 domains.yaml 路径（同 scheduler 的 config/pkb 约定）。
func NewPKBSteerHandler(basePath, domainsPath string) *PKBSteerHandler {
	return &PKBSteerHandler{basePath: basePath, domainsPath: domainsPath}
}

// proposalDTO 是待批骨架提议的 JSON 投影（snake_case，前端消费；与内部 pkb.SkeletonProposal 解耦）。
type proposalDTO struct {
	ID           string `json:"id"`
	Domain       string `json:"domain"`
	Action       string `json:"action"`
	Summary      string `json:"summary"`
	ImpactRadius int    `json:"impact_radius"`
	ProposedTree string `json:"proposed_tree"`
	VaultSubpath string `json:"vault_subpath"`
}

// Proposals GET /api/pkb/proposals —— 列出全部待批骨架「大动作」提议（超影响半径阈值需人批准的）。
func (h *PKBSteerHandler) Proposals(c *gin.Context) {
	props, err := pkb.ListPendingProposals(h.basePath)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	out := make([]proposalDTO, 0, len(props))
	for _, p := range props {
		out = append(out, proposalDTO{
			ID:           p.ID,
			Domain:       p.Domain,
			Action:       p.Action,
			Summary:      p.Summary,
			ImpactRadius: p.ImpactRadius,
			ProposedTree: p.ProposedTree,
			VaultSubpath: p.VaultSubpath,
		})
	}
	response.Success(c, out)
}

// ApproveProposal POST /api/pkb/proposals/:id/approve —— 应用提议：快照旧 _index.md → 用提议树
// 替换「## 知识树」→ 删提议文件。卡片归位留待下轮 digest/match（与 Matrix !pkb approve 同一路径）。
func (h *PKBSteerHandler) ApproveProposal(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		response.BadRequest(c, "缺少提议 id")
		return
	}
	msg, err := pkb.ApplySkeletonProposal(h.basePath, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": msg})
}

// RejectProposal POST /api/pkb/proposals/:id/reject —— 驳回提议：删提议文件，骨架不动。
func (h *PKBSteerHandler) RejectProposal(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		response.BadRequest(c, "缺少提议 id")
		return
	}
	msg, err := pkb.RejectSkeletonProposal(h.basePath, id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": msg})
}

// domainDTO 是领域大方向的 JSON 投影：can_set_scope 标识该领域能否设 scope
// （资讯流 feed / 兜底 is_default 域不生成骨架、不可设）。
type domainDTO struct {
	Name         string `json:"name"`
	Display      string `json:"display"`
	Scope        string `json:"scope"`
	VaultSubpath string `json:"vault_subpath"`
	Feed         bool   `json:"feed"`
	IsDefault    bool   `json:"is_default"`
	CanSetScope  bool   `json:"can_set_scope"`
}

// Domains GET /api/pkb/domains —— 列出全部领域及其当前大方向(scope)，供掌舵面「设领域大方向」节。
func (h *PKBSteerHandler) Domains(c *gin.Context) {
	dc, err := pkb.LoadDomains(h.domainsPath)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	out := make([]domainDTO, 0, len(dc.Domains))
	for _, d := range dc.Domains {
		out = append(out, domainDTO{
			Name:         d.Name,
			Display:      d.Display,
			Scope:        d.Scope,
			VaultSubpath: d.VaultSubpath,
			Feed:         d.Feed,
			IsDefault:    d.IsDefault,
			CanSetScope:  !d.Feed && !d.IsDefault,
		})
	}
	response.Success(c, out)
}

// setScopeRequest 是 PUT /domains/:name/scope 的请求体。
type setScopeRequest struct {
	Scope string `json:"scope"`
}

// SetScope PUT /api/pkb/domains/:name/scope —— 改某领域的一句话大方向，外科式写回 domains.yaml
// （保注释）。资讯流/兜底域会被拒。改完下次 pkb-curate skeleton 运行即生效。
func (h *PKBSteerHandler) SetScope(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		response.BadRequest(c, "缺少领域 name")
		return
	}
	var req setScopeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体应为 {\"scope\": \"...\"}")
		return
	}
	if err := pkb.SetDomainScope(h.domainsPath, name, req.Scope); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "✅ 领域 " + name + " 大方向已更新，下次 pkb-curate skeleton 运行即生效"})
}

// createDomainRequest 是 POST /domains 的请求体（最小字段）。
type createDomainRequest struct {
	Display string `json:"display"`
	Scope   string `json:"scope"`
}

// CreateDomain POST /api/pkb/domains —— 新建知识领域（最小字段 display+scope，name/vault_subpath 派生）。
// 不自动生成骨架，由前端「生成骨架」或下次自动维护播种。
func (h *PKBSteerHandler) CreateDomain(c *gin.Context) {
	var req createDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体应为 {\"display\":\"...\",\"scope\":\"...\"}")
		return
	}
	if err := pkb.AddDomain(h.domainsPath, req.Display, req.Scope); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "✅ 领域「" + req.Display + "」已创建，可点「生成骨架」播种或等自动维护"})
}

// DeleteDomain DELETE /api/pkb/domains/:name —— 删除领域配置条目（vault 文件保留，浏览归 Obsidian）。
func (h *PKBSteerHandler) DeleteDomain(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		response.BadRequest(c, "缺少领域 name")
		return
	}
	if err := pkb.DeleteDomain(h.domainsPath, name); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "🗑 领域 " + name + " 配置已删除（vault 文件保留）"})
}

// setDisplayRequest 是 PUT /domains/:name/display 的请求体。
type setDisplayRequest struct {
	Display string `json:"display"`
}

// SetDisplay PUT /api/pkb/domains/:name/display —— 改领域显示名（仅 display，不动 name/路径/分类）。
func (h *PKBSteerHandler) SetDisplay(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		response.BadRequest(c, "缺少领域 name")
		return
	}
	var req setDisplayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体应为 {\"display\":\"...\"}")
		return
	}
	if err := pkb.SetDomainDisplay(h.domainsPath, name, req.Display); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "✅ 领域 " + name + " 显示名已更新为「" + req.Display + "」"})
}

// DomainStats GET /api/pkb/domains/stats —— 各领域状态概览（卡片数/当天·近7天新增/缺口/待归位/
// 低置信/最近 digest/是否有骨架），供前端「知识骨架」总览。纯只读，不调 LLM。
func (h *PKBSteerHandler) DomainStats(c *gin.Context) {
	stats, err := pkb.DomainStatsOverview(h.basePath, h.domainsPath)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	// 据 scheduler 队列标记「排队中/生成中」，供前端即时反馈（点完即见「排队中」）。
	if h.skeletonStatus != nil {
		queued, running := h.skeletonStatus()
		pending := make(map[string]bool, len(queued)+1)
		for _, d := range queued {
			pending[d] = true
		}
		if running != "" {
			pending[running] = true
		}
		for i := range stats {
			if pending[stats[i].Name] {
				stats[i].SkeletonPending = true
			}
		}
	}
	// 据 config 阈值标「需要关注」（待归位/低置信偏高），供总览「需要关注」块直达对应领域。
	for i := range stats {
		if h.waitlistHigh > 0 && stats[i].Waitlist >= h.waitlistHigh {
			stats[i].WaitlistHigh = true
		}
		if h.lowConfHigh > 0 && stats[i].LowConfidence >= h.lowConfHigh {
			stats[i].LowConfidenceHigh = true
		}
	}
	response.Success(c, stats)
}

// GenerateSkeleton POST /api/pkb/domains/:name/skeleton —— 后台异步为某领域生成/重建骨架。
// skeleton 调顶级模型画树（约 1-3 分钟），故异步触发、立即返回，结果经 domains/stats 反映。
func (h *PKBSteerHandler) GenerateSkeleton(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		response.BadRequest(c, "缺少领域 name")
		return
	}
	dc, err := pkb.LoadDomains(h.domainsPath)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	var dom *pkb.Domain
	for i := range dc.Domains {
		if dc.Domains[i].Name == name {
			dom = &dc.Domains[i]
			break
		}
	}
	if dom == nil {
		response.BadRequest(c, "领域 "+name+" 不存在")
		return
	}
	if dom.Feed || dom.IsDefault {
		response.BadRequest(c, "领域 "+name+" 不生成骨架（资讯流/兜底域）")
		return
	}
	if strings.TrimSpace(dom.Scope) == "" {
		response.BadRequest(c, "领域 "+name+" 未设 scope，先设大方向再生成骨架")
		return
	}
	if h.skeletonRunner == nil {
		response.InternalError(c, "骨架生成器未就绪（PKB 调度器未启用）")
		return
	}
	if err := h.skeletonRunner(name); err != nil {
		response.InternalError(c, "排队失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "⏳ 已排队生成「" + name + "」骨架，将在当前任务完成后优先执行（先于自动任务），稍后刷新状态查看"})
}
