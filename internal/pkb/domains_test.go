package pkb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleDomainsYAML 带密集注释的 domains.yaml 样本：含有 scope 的知识域、无 scope 的知识域、
// 兜底域(is_default)、资讯流域(feed)，用于验证 SetDomainScope 外科式替换保注释/拒特殊域。
const sampleDomainsYAML = `# 顶部注释——不应被改动
defaults:
  vault_threshold: 7.0   # 行内注释
  score_model: pool-summary

# 领域清单注释
domains:
  - name: programming
    display: 编程
    scope: 旧的编程大方向
    vault_subpath: vault/编程
    keywords: [go, rust]

  - name: cs-fundamentals
    display: 计算机基础
    vault_subpath: vault/基础
    keywords: [算法]

  - name: misc
    display: 杂项
    vault_subpath: vault/杂项
    is_default: true

  - name: news
    display: 资讯
    vault_subpath: vault/资讯
    feed: true
`

func writeSampleDomains(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.yaml")
	if err := os.WriteFile(path, []byte(sampleDomainsYAML), 0644); err != nil {
		t.Fatalf("写样本 domains.yaml: %v", err)
	}
	return path
}

func loadScope(t *testing.T, path, name string) string {
	t.Helper()
	dc, err := LoadDomains(path)
	if err != nil {
		t.Fatalf("LoadDomains: %v", err)
	}
	for _, d := range dc.Domains {
		if d.Name == name {
			return d.Scope
		}
	}
	t.Fatalf("领域 %s 未找到", name)
	return ""
}

func TestSetDomainScope_ReplaceExisting(t *testing.T) {
	path := writeSampleDomains(t)
	want := "后端与全栈工程——Go/Rust/C# 与微服务架构。"
	if err := SetDomainScope(path, "programming", want); err != nil {
		t.Fatalf("SetDomainScope: %v", err)
	}
	if got := loadScope(t, path, "programming"); got != want {
		t.Fatalf("scope 未更新：want %q got %q", want, got)
	}
	// 注释与其它领域必须保留。
	data, _ := os.ReadFile(path)
	content := string(data)
	for _, must := range []string{"# 顶部注释——不应被改动", "# 领域清单注释", "# 行内注释", "name: cs-fundamentals", "name: news"} {
		if !strings.Contains(content, must) {
			t.Fatalf("回写后丢失内容：%q\n---\n%s", must, content)
		}
	}
	// 旧 scope 不应残留。
	if strings.Contains(content, "旧的编程大方向") {
		t.Fatalf("旧 scope 仍残留\n---\n%s", content)
	}
}

func TestSetDomainScope_InsertWhenMissing(t *testing.T) {
	path := writeSampleDomains(t)
	want := "算法与数据结构、操作系统、网络协议。"
	if err := SetDomainScope(path, "cs-fundamentals", want); err != nil {
		t.Fatalf("SetDomainScope: %v", err)
	}
	if got := loadScope(t, path, "cs-fundamentals"); got != want {
		t.Fatalf("scope 未插入：want %q got %q", want, got)
	}
	// 其它领域 scope 不受影响。
	if got := loadScope(t, path, "programming"); got != "旧的编程大方向" {
		t.Fatalf("programming scope 被误改：%q", got)
	}
	// 插入位置应在 display 之后、紧邻块内（保持 name/display/scope 序）。
	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\n")
	dispIdx, scopeIdx := -1, -1
	for i, l := range lines {
		tl := strings.TrimSpace(l)
		if tl == "name: cs-fundamentals" || tl == "- name: cs-fundamentals" {
			// 找该块内的 display / scope
			for j := i + 1; j < len(lines); j++ {
				tj := strings.TrimSpace(lines[j])
				if strings.HasPrefix(tj, "- name:") {
					break
				}
				if strings.HasPrefix(tj, "display:") {
					dispIdx = j
				}
				if strings.HasPrefix(tj, "scope:") {
					scopeIdx = j
				}
			}
			break
		}
	}
	if dispIdx < 0 || scopeIdx < 0 || scopeIdx != dispIdx+1 {
		t.Fatalf("scope 未紧跟 display 插入：dispIdx=%d scopeIdx=%d", dispIdx, scopeIdx)
	}
}

func TestSetDomainScope_RejectFeedAndDefault(t *testing.T) {
	path := writeSampleDomains(t)
	before, _ := os.ReadFile(path)
	for _, name := range []string{"news", "misc"} {
		if err := SetDomainScope(path, name, "不该被接受的 scope"); err == nil {
			t.Fatalf("领域 %s 应拒绝设 scope，但成功了", name)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("拒绝后文件不应改动")
	}
}

func TestSetDomainScope_QuotesUnsafe(t *testing.T) {
	path := writeSampleDomains(t)
	// 含 `: `（冒号空格）必须走引号，否则 plain 解析会断成映射。
	want := "原理: 方法与模式 #1"
	if err := SetDomainScope(path, "programming", want); err != nil {
		t.Fatalf("SetDomainScope: %v", err)
	}
	if got := loadScope(t, path, "programming"); got != want {
		t.Fatalf("含特殊字符 scope 往返失真：want %q got %q", want, got)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `scope: "原理: 方法与模式 #1"`) {
		t.Fatalf("不安全 scope 未走双引号渲染\n---\n%s", string(data))
	}
}

func TestSetDomainScope_Errors(t *testing.T) {
	path := writeSampleDomains(t)
	if err := SetDomainScope(path, "nonexistent", "x"); err == nil {
		t.Fatalf("不存在领域应报错")
	}
	if err := SetDomainScope(path, "programming", "   "); err == nil {
		t.Fatalf("空 scope 应报错")
	}
	if err := SetDomainScope(path, "programming", "多\n行"); err == nil {
		t.Fatalf("多行 scope 应报错")
	}
}

func TestIsPlainSafeYAML(t *testing.T) {
	plain := []string{"后端与全栈工程实践——主流语言（Java/C#/.NET/Go/Rust）。", "simple", "a/b-c"}
	for _, s := range plain {
		if !isPlainSafeYAML(s) {
			t.Errorf("应为 plain-safe：%q", s)
		}
	}
	unsafe := []string{"", " 前导空格", "尾随冒号:", "键: 值", "有 #注释", `含"引号`, "- 前导横线"}
	for _, s := range unsafe {
		if isPlainSafeYAML(s) {
			t.Errorf("应判为需引号：%q", s)
		}
	}
}

func TestAddDomain(t *testing.T) {
	path := writeSampleDomains(t)
	if err := AddDomain(path, "区块链", "区块链与智能合约——共识机制、EVM、DeFi 与链上安全。"); err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	dc, err := LoadDomains(path)
	if err != nil {
		t.Fatalf("LoadDomains: %v", err)
	}
	var d *Domain
	for i := range dc.Domains {
		if dc.Domains[i].Name == "区块链" {
			d = &dc.Domains[i]
		}
	}
	if d == nil {
		t.Fatal("新域未加入")
	}
	if d.Display != "区块链" || d.VaultSubpath != "vault/区块链" || strings.TrimSpace(d.Scope) == "" {
		t.Fatalf("新域字段不符: %+v", d)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# 顶部注释——不应被改动") {
		t.Fatal("回写后注释丢失")
	}
	if err := AddDomain(path, "区块链", "x"); err == nil {
		t.Fatal("重复 display 应拒绝")
	}
	if err := AddDomain(path, "新域X", ""); err == nil {
		t.Fatal("空 scope 应拒绝")
	}
	if err := AddDomain(path, "a/b", "x"); err == nil {
		t.Fatal("含非法字符应拒绝")
	}
}

func TestDeleteDomain(t *testing.T) {
	path := writeSampleDomains(t)
	before, _ := LoadDomains(path)
	if err := DeleteDomain(path, "cs-fundamentals"); err != nil {
		t.Fatalf("DeleteDomain: %v", err)
	}
	after, _ := LoadDomains(path)
	if len(after.Domains) != len(before.Domains)-1 {
		t.Fatalf("领域数未减 1：%d→%d", len(before.Domains), len(after.Domains))
	}
	for _, d := range after.Domains {
		if d.Name == "cs-fundamentals" {
			t.Fatal("目标域仍在")
		}
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "name: programming") || !strings.Contains(string(data), "# 顶部注释——不应被改动") {
		t.Fatal("误删了其它领域或注释")
	}
	if err := DeleteDomain(path, "misc"); err == nil {
		t.Fatal("应拒删兜底域(is_default)")
	}
	if err := DeleteDomain(path, "news"); err == nil {
		t.Fatal("应拒删资讯流域(feed)")
	}
	if err := DeleteDomain(path, "nope"); err == nil {
		t.Fatal("不存在领域应报错")
	}
}

func TestSetDomainDisplay(t *testing.T) {
	path := writeSampleDomains(t)
	if err := SetDomainDisplay(path, "programming", "后端开发"); err != nil {
		t.Fatalf("SetDomainDisplay: %v", err)
	}
	dc, _ := LoadDomains(path)
	for _, d := range dc.Domains {
		if d.Name == "programming" {
			if d.Display != "后端开发" {
				t.Fatalf("display 未改: %q", d.Display)
			}
			if d.VaultSubpath != "vault/编程" {
				t.Fatalf("vault_subpath 不应随 display 改变: %q", d.VaultSubpath)
			}
			if strings.TrimSpace(d.Scope) != "旧的编程大方向" {
				t.Fatalf("scope 不应被动: %q", d.Scope)
			}
		}
	}
	if err := SetDomainDisplay(path, "cs-fundamentals", "后端开发"); err == nil {
		t.Fatal("重复 display 应拒绝")
	}
	if err := SetDomainDisplay(path, "nope", "x"); err == nil {
		t.Fatal("不存在领域应报错")
	}
}
