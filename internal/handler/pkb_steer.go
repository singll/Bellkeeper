package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkb"
	"github.com/singll/bellkeeper/internal/pkg/response"
)

// PKBSteerHandler 知识骨架「调方向」掌舵面（Phase I / ADR-0004 Q9/Q12）：把原本只能走
// Matrix !pkb / CLI 的「待批骨架提议审批」与「领域大方向(scope)设定」搬到前端 REST，
// 让人用窄掌舵面调方向。浏览仍归 Obsidian、骨架文件仍机器独占写（W1），本 handler 不做文件编辑。
type PKBSteerHandler struct {
	basePath    string // vault 根（待批提议落 basePath/_提议/<id>.md）
	domainsPath string // config/pkb/domains.yaml（设 scope 的落点）
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
