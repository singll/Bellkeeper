package pkb

import "testing"

// TestFrontmatterValue 验证自愈判断依赖的 frontmatter 解析：
// 决定一篇 raw 是否「上轮已处理」全靠它正确读出 pkb_decision，解析错会导致重复打分。
func TestFrontmatterValue(t *testing.T) {
	cases := []struct {
		name    string
		content string
		key     string
		want    string
	}{
		{"无 frontmatter", "# Title\n正文", "pkb_decision", ""},
		{"key 存在", "---\ntitle: x\npkb_decision: vault\n---\n正文", "pkb_decision", "vault"},
		{"key 不存在", "---\ntitle: x\n---\n正文", "pkb_decision", ""},
		{"值含多余空白被裁剪", "---\npkb_decision:   archive  \n---\n", "pkb_decision", "archive"},
		{"frontmatter 未闭合仍能提取", "---\npkb_decision: discard\n", "pkb_decision", "discard"},
		{"空内容", "", "pkb_decision", ""},
		{"首行非分隔符", "pkb_decision: vault\n", "pkb_decision", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := frontmatterValue(tc.content, tc.key); got != tc.want {
				t.Errorf("frontmatterValue(%q, %q) = %q, want %q", tc.content, tc.key, got, tc.want)
			}
		})
	}
}
