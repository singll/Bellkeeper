package pkb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestComputeAuditMetrics_LowConfidence 守住 §6 低置信卡指标：
// verification ∈ {unverified, llm-only} 计入低置信；verified/空与非卡节点不计。
func TestComputeAuditMetrics_LowConfidence(t *testing.T) {
	g := &auditGraph{
		nodes: map[string]*auditNode{
			"a.md":   {AtomicConcept: "A", Type: "pkb_card", Verification: "verified"},
			"b.md":   {AtomicConcept: "B", Type: "pkb_card", Verification: "unverified"},
			"c.md":   {AtomicConcept: "C", Type: "pkb_card", Verification: "llm-only"},
			"d.md":   {AtomicConcept: "D", Type: "pkb_card", Verification: ""},
			"map.md": {AtomicConcept: "M", Type: "pkb_map", Verification: ""}, // 非卡，不计入 TotalCards
		},
		conceptIndex: map[string][]string{},
		validTargets: map[string]bool{},
	}
	r := g.computeAuditMetrics()
	assert.Equal(t, 4, r.TotalCards)         // 4 张 pkb_card（pkb_map 不计）
	assert.Equal(t, 2, r.LowConfidenceCards) // unverified + llm-only
	assert.InDelta(t, 50.0, r.LowConfidenceRate, 0.01)
}
