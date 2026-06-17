package pkb

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// AuditResult 网健康度审计结果
type AuditResult struct {
	TotalCards        int              `json:"total_cards"`
	OrphanCards       int              `json:"orphan_cards"`
	OrphanRate        float64          `json:"orphan_rate"`
	BrokenLinks       int              `json:"broken_links"`
	AvgDegree         float64          `json:"avg_degree"`
	HubCards          []HubCard        `json:"hub_cards"`
	DuplicateConcepts []ConceptCluster `json:"duplicate_concepts"`
	CoverageRate      float64          `json:"coverage_rate"`
	CardsWithMOC      int              `json:"cards_with_moc"`
	// 低置信卡（缺口填充未核实/纯 LLM 编写）：verification ∈ {unverified, llm-only}（计划 §6 / Phase G）。
	LowConfidenceCards int     `json:"low_confidence_cards"`
	LowConfidenceRate  float64 `json:"low_confidence_rate"`
}

// HubCard 超级 hub 卡片（被过度连接的节点）
type HubCard struct {
	Concept   string `json:"concept"`
	FilePath  string `json:"file_path"`
	InDegree  int    `json:"in_degree"`
	OutDegree int    `json:"out_degree"`
}

// ConceptCluster 同名/近名 concept 簇
type ConceptCluster struct {
	Concept string   `json:"concept"`
	Files   []string `json:"files"`
}

type auditNode struct {
	FilePath      string
	AtomicConcept string
	Aliases       []string
	Type          string
	Verification  string
	InDegree      int
	OutDegree     int
	OutLinks      []string
}

// auditGraph 包含建图后的节点和辅助索引
type auditGraph struct {
	nodes        map[string]*auditNode
	conceptIndex map[string][]string
	validTargets map[string]bool
}

// buildAuditGraph 扫描 vault 构建链接图（公共函数，供 RunAudit 和 AuditSummary 复用）。
func (c *Curator) buildAuditGraph() *auditGraph {
	nodes := make(map[string]*auditNode)
	conceptIndex := make(map[string][]string)

	for _, domain := range c.domains.Domains {
		root := filepath.Join(c.basePath, domain.VaultSubpath)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(d.Name()) != ".md" {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			content := string(data)
			fm := parseFrontmatterMap(content)
			nodeType := fm["type"]
			verification := fm["verification"]
			concept := fm["atomic_concept"]
			if concept == "" {
				concept = fm["title"]
			}
			if concept == "" {
				concept = strings.TrimSuffix(d.Name(), ".md")
			}

			var aliases []string
			if raw := fm["aliases"]; raw != "" {
				raw = strings.Trim(raw, "[]")
				for _, a := range strings.Split(raw, ",") {
					a = strings.TrimSpace(strings.Trim(a, `"'`))
					if a != "" {
						aliases = append(aliases, a)
					}
				}
			}

			links := extractWikilinks(stripFrontmatter(content))
			node := &auditNode{
				FilePath:      path,
				AtomicConcept: concept,
				Aliases:       aliases,
				Type:          nodeType,
				Verification:  verification,
				OutLinks:      links,
			}
			nodes[path] = node

			if concept != "" {
				conceptIndex[concept] = append(conceptIndex[concept], path)
			}
			return nil
		}); err != nil {
			log.Printf("[PKB] failed to walk directory %s: %v", root, err)
		}
	}

	validTargets := make(map[string]bool)
	for _, node := range nodes {
		validTargets[node.AtomicConcept] = true
		for _, a := range node.Aliases {
			validTargets[a] = true
		}
	}

	return &auditGraph{
		nodes:        nodes,
		conceptIndex: conceptIndex,
		validTargets: validTargets,
	}
}

// computeAuditMetrics 在已建图上计算所有审计指标（两轮：先建边，再判孤儿）。
// 孤儿口径统一：InDegree==0 && OutDegree==0（OutDegree 只计有效链接）。
func (g *auditGraph) computeAuditMetrics() AuditResult {
	var result AuditResult

	// 第一轮：建边，计算 InDegree/OutDegree/BrokenLinks
	totalDegree := 0
	for _, node := range g.nodes {
		if node.Type != "pkb_card" && node.Type != "" {
			continue
		}
		result.TotalCards++
		if isLowConfidence(node.Verification) {
			result.LowConfidenceCards++
		}
		outDegree := 0
		for _, link := range node.OutLinks {
			if g.validTargets[link] {
				outDegree++
			} else {
				result.BrokenLinks++
			}
			targetPath := g.resolveLink(link)
			if targetPath != "" && g.nodes[targetPath] != nil {
				g.nodes[targetPath].InDegree++
			}
		}
		node.OutDegree = outDegree
		totalDegree += outDegree
	}

	// 第二轮：判孤儿（此时所有 InDegree 已累加完毕）
	for _, node := range g.nodes {
		if node.Type != "pkb_card" && node.Type != "" {
			continue
		}
		if node.InDegree == 0 && node.OutDegree == 0 {
			result.OrphanCards++
		}
	}

	if result.TotalCards > 0 {
		result.OrphanRate = float64(result.OrphanCards) / float64(result.TotalCards) * 100
		result.AvgDegree = float64(totalDegree) / float64(result.TotalCards)
		result.LowConfidenceRate = float64(result.LowConfidenceCards) / float64(result.TotalCards) * 100
	}

	for concept, files := range g.conceptIndex {
		if len(files) > 1 {
			result.DuplicateConcepts = append(result.DuplicateConcepts, ConceptCluster{
				Concept: concept,
				Files:   files,
			})
		}
	}

	mocCovered := make(map[string]bool)
	for _, node := range g.nodes {
		if node.Type == "pkb_map" || node.Type == "pkb_topic" {
			for _, link := range node.OutLinks {
				mocCovered[link] = true
			}
		}
	}
	for _, node := range g.nodes {
		if node.Type != "pkb_card" && node.Type != "" {
			continue
		}
		if mocCovered[node.AtomicConcept] {
			result.CardsWithMOC++
		}
	}
	if result.TotalCards > 0 {
		result.CoverageRate = float64(result.CardsWithMOC) / float64(result.TotalCards) * 100
	}

	for _, node := range g.nodes {
		total := node.InDegree + node.OutDegree
		if total >= 10 {
			result.HubCards = append(result.HubCards, HubCard{
				Concept:   node.AtomicConcept,
				FilePath:  node.FilePath,
				InDegree:  node.InDegree,
				OutDegree: node.OutDegree,
			})
		}
	}

	return result
}

func (g *auditGraph) resolveLink(link string) string {
	for _, node := range g.nodes {
		if node.AtomicConcept == link {
			return node.FilePath
		}
		for _, a := range node.Aliases {
			if a == link {
				return node.FilePath
			}
		}
	}
	return ""
}

// RunAudit 扫描 vault 构建链接图，输出网健康度报告。只读，不写盘。
func (c *Curator) RunAudit(jsonOutput bool) error {
	g := c.buildAuditGraph()
	result := g.computeAuditMetrics()

	if jsonOutput {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		printAuditReport(result)
	}
	return nil
}

func extractWikilinks(body string) []string {
	matches := wikilinkRe.FindAllStringSubmatch(body, -1)
	var links []string
	for _, m := range matches {
		if len(m) > 1 {
			target := strings.TrimSpace(strings.SplitN(m[1], "|", 2)[0])
			links = append(links, target)
		}
	}
	return links
}

func printAuditReport(r AuditResult) {
	fmt.Println("[pkb-audit] 网健康度报告")
	fmt.Printf("  总卡片数: %d\n", r.TotalCards)
	fmt.Printf("  孤儿卡: %d (%.1f%%)\n", r.OrphanCards, r.OrphanRate)
	fmt.Printf("  断链数: %d\n", r.BrokenLinks)
	fmt.Printf("  低置信卡: %d (%.1f%%) [unverified+llm-only]\n", r.LowConfidenceCards, r.LowConfidenceRate)
	fmt.Printf("  平均度: %.1f\n", r.AvgDegree)
	fmt.Printf("  MOC 覆盖率: %.1f%% (%d/%d)\n", r.CoverageRate, r.CardsWithMOC, r.TotalCards)
	if len(r.DuplicateConcepts) > 0 {
		fmt.Printf("  重复概念: %d 簇\n", len(r.DuplicateConcepts))
		for _, cluster := range r.DuplicateConcepts {
			fmt.Printf("    - %s: %d 卡\n", cluster.Concept, len(cluster.Files))
		}
	}
	if len(r.HubCards) > 0 {
		fmt.Printf("  超级 hub (度≥10): %d\n", len(r.HubCards))
		for _, hub := range r.HubCards {
			fmt.Printf("    - %s: 入=%d 出=%d\n", hub.Concept, hub.InDegree, hub.OutDegree)
		}
	}
}

// AuditSummary 返回一行精简摘要，供 pkb-curate Run() 末尾输出。
func (c *Curator) AuditSummary() string {
	g := c.buildAuditGraph()
	result := g.computeAuditMetrics()
	return fmt.Sprintf("孤儿=%d 断链=%d 低置信=%d 总卡=%d", result.OrphanCards, result.BrokenLinks, result.LowConfidenceCards, result.TotalCards)
}
