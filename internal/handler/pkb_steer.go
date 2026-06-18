package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkb"
	"github.com/singll/bellkeeper/internal/pkg/response"
)

// PKBSteerHandler 知识骨架「调方向」掌舵面（Phase I / ADR-0004 Q9/Q12）：把原本只能走
// Matrix !pkb / CLI 的「待批骨架提议审批」搬到前端 REST，让人用窄掌舵面调方向。
// 浏览仍归 Obsidian、骨架文件仍机器独占写（W1），本 handler 不做文件编辑。
type PKBSteerHandler struct {
	basePath string // vault 根（待批提议落 basePath/_提议/<id>.md）
}

// NewPKBSteerHandler 构造掌舵面 handler。basePath 为 vault 根（同 Matrix !pkb 闭包取值）。
func NewPKBSteerHandler(basePath string) *PKBSteerHandler {
	return &PKBSteerHandler{basePath: basePath}
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
