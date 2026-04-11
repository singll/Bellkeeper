package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// DirectMemosHandler handles Memos todo directly with todo.txt format
type DirectMemosHandler struct {
	BaseHandler
	apiURL     string
	apiToken   string
	httpClient *http.Client
}

// NewDirectMemosHandler creates a direct Memos handler
func NewDirectMemosHandler(apiURL, apiToken string) *DirectMemosHandler {
	return &DirectMemosHandler{
		BaseHandler: BaseHandler{
			name:        "待办",
			description: "Memos 待办管理（支持 todo.txt 格式）",
			usage:       "<子命令> [参数]",
		},
		apiURL:   apiURL,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (h *DirectMemosHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	if len(cmdCtx.Command.Argv) == 0 {
		return &Response{
			Success: true,
			Message: "**📋 待办命令（todo.txt 格式）**\n\n" +
				"`!待办 列表` - 查看所有待办\n" +
				"`!待办 新增 <内容>` - 添加待办\n" +
				"`!待办 完成 <ID>` - 标记完成\n" +
				"`!待办 删除 <ID>` - 删除待办\n\n" +
				"**参数格式示例：**\n" +
				"`P1 D4/20 完成报告` - P1优先级, 4月20日截止\n" +
				"`+工作 @办公室 任务` - +项目名, @上下文\n" +
				"`P2 +项目 @上下文 D5/1 内容` - 组合使用",
			IsHTML: true,
		}, nil
	}

	subCmd := cmdCtx.Command.Argv[0]
	args := ""
	if len(cmdCtx.Command.Argv) > 1 {
		args = cmdCtx.Command.Args[len(subCmd)+1:]
	}

	switch subCmd {
	case "列表", "list":
		return h.listTodos(ctx)
	case "新增", "add", "new":
		return h.addTodo(ctx, args)
	case "完成", "done":
		return h.completeTodo(ctx, args)
	case "删除", "delete", "del":
		return h.deleteTodo(ctx, args)
	case "显示", "show":
		return h.showTodo(ctx, args)
	default:
		return &Response{
			Success: false,
			Message: fmt.Sprintf("未知子命令: %s", subCmd),
		}, nil
	}
}

func (h *DirectMemosHandler) listTodos(ctx context.Context) (*Response, error) {
	if h.apiURL == "" || h.apiToken == "" {
		return &Response{
			Success: false,
			Message: "Memos API 未配置，请联系管理员",
		}, nil
	}

	url := h.apiURL + "/api/v1/memos"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 调用 Memos API 失败: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ Memos API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody)),
		}, nil
	}

	// Parse response
	var result struct {
		Memos []struct {
			ID          int    `json:"id"`
			Content     string `json:"content"`
			CreatedTs   int64  `json:"createdTs"`
			Done        bool   `json:"done"`
			CompletedTs *int64 `json:"completedTs"`
			Tags        []struct {
				Name string `json:"name"`
			} `json:"tags"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 解析 Memos 响应失败: %v", err),
		}, nil
	}

	// Convert to todo items
	var todoItems []*TodoItem
	for _, m := range result.Memos {
		// Check if it has a todo-related tag
		isTodo := false
		for _, t := range m.Tags {
			if t.Name == "todo" || t.Name == "待办" || t.Name == "任务" {
				isTodo = true
				break
			}
		}
		if !isTodo && len(m.Tags) == 0 {
			isTodo = true
		}

		if !isTodo {
			continue
		}

		item := &TodoItem{
			ID:         m.ID,
			Done:       m.Done,
			RawContent: m.Content,
		}

		// Parse created date
		if m.CreatedTs > 0 {
			item.CreatedAt = formatTimestamp(m.CreatedTs)
		}

		// Parse completed date
		if m.CompletedTs != nil && *m.CompletedTs > 0 {
			item.CompletedAt = formatTimestamp(*m.CompletedTs)
		}

		// Convert Memos content to todo.txt format for display
		item.RawContent = MemosToTodoTxt(m.Content)

		todoItems = append(todoItems, item)
	}

	// Format and return
	formatted := FormatTodoList(todoItems, false)
	formatted += "\n\n💡 使用 `!待办 完成 <ID>` 标记完成"

	return &Response{
		Success: true,
		Message: formatted,
		IsHTML:  true,
	}, nil
}

func (h *DirectMemosHandler) addTodo(ctx context.Context, content string) (*Response, error) {
	if content == "" {
		return &Response{
			Success: false,
			Message: "请输入待办内容\n格式: `!待办 新增 P1 D4/20 内容 +项目 @上下文`",
		}, nil
	}

	if h.apiURL == "" || h.apiToken == "" {
		return &Response{
			Success: false,
			Message: "Memos API 未配置，请联系管理员",
		}, nil
	}

	// Parse command input to extract parameters
	params := ParseCommandInput(content)

	// Build Memos content
	memosContent := params.ToMemosContent()

	url := h.apiURL + "/api/v1/memos"
	payload := map[string]interface{}{
		"content":    memosContent,
		"visibility": "PRIVATE",
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 创建待办失败: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ Memos API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody)),
		}, nil
	}

	// Parse response to get ID
	var result struct {
		Memo struct {
			ID int `json:"id"`
		} `json:"memo"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.Memo.ID > 0 {
		// Format display for todo.txt
		displayTxt := params.ToTodoTxtDisplay()
		return &Response{
			Success: true,
			Message: fmt.Sprintf("✅ 已添加待办\n`%s`\n\n[#%d]", displayTxt, result.Memo.ID),
		}, nil
	}

	return &Response{
		Success: true,
		Message: fmt.Sprintf("✅ 已添加待办: %s", content),
	}, nil
}

func (h *DirectMemosHandler) completeTodo(ctx context.Context, idStr string) (*Response, error) {
	if idStr == "" {
		return &Response{
			Success: false,
			Message: "请提供待办 ID，例如: `!待办 完成 123`",
		}, nil
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("无效的 ID: %s", idStr),
		}, nil
	}

	if h.apiURL == "" || h.apiToken == "" {
		return &Response{
			Success: false,
			Message: "Memos API 未配置，请联系管理员",
		}, nil
	}

	url := h.apiURL + "/api/v1/memos/" + strconv.Itoa(id)
	payload := map[string]interface{}{
		"done": true,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 标记完成失败: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ Memos API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody)),
		}, nil
	}

	return &Response{
		Success: true,
		Message: fmt.Sprintf("✅ 已完成待办 [#%d]", id),
	}, nil
}

func (h *DirectMemosHandler) deleteTodo(ctx context.Context, idStr string) (*Response, error) {
	if idStr == "" {
		return &Response{
			Success: false,
			Message: "请提供待办 ID，例如: `!待办 删除 123`",
		}, nil
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("无效的 ID: %s", idStr),
		}, nil
	}

	if h.apiURL == "" || h.apiToken == "" {
		return &Response{
			Success: false,
			Message: "Memos API 未配置，请联系管理员",
		}, nil
	}

	url := h.apiURL + "/api/v1/memos/" + strconv.Itoa(id)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiToken)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 删除待办失败: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		respBody, _ := io.ReadAll(resp.Body)
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ Memos API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody)),
		}, nil
	}

	return &Response{
		Success: true,
		Message: fmt.Sprintf("🗑️ 已删除待办 [#%d]", id),
	}, nil
}

func (h *DirectMemosHandler) showTodo(ctx context.Context, idStr string) (*Response, error) {
	if idStr == "" {
		return &Response{
			Success: false,
			Message: "请提供待办 ID，例如: `!待办 显示 123`",
		}, nil
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("无效的 ID: %s", idStr),
		}, nil
	}

	if h.apiURL == "" || h.apiToken == "" {
		return &Response{
			Success: false,
			Message: "Memos API 未配置，请联系管理员",
		}, nil
	}

	url := h.apiURL + "/api/v1/memos/" + strconv.Itoa(id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiToken)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 查询待办失败: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ Memos API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody)),
		}, nil
	}

	var memo struct {
		Memo struct {
			ID          int    `json:"id"`
			Content     string `json:"content"`
			CreatedTs   int64  `json:"createdTs"`
			Done        bool   `json:"done"`
			CompletedTs *int64 `json:"completedTs"`
		} `json:"memo"`
	}

	if err := json.Unmarshal(respBody, &memo); err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 解析响应失败: %v", err),
		}, nil
	}

	todoTxt := MemosToTodoTxt(memo.Memo.Content)

	status := "📝 进行中"
	if memo.Memo.Done {
		status = "✅ 已完成"
	}

	return &Response{
		Success: true,
		Message: fmt.Sprintf("**#%d** %s\n\n`%s`\n\n创建时间: %s",
			memo.Memo.ID, status, todoTxt, formatTimestamp(memo.Memo.CreatedTs)),
	}, nil
}

// Helper function to format Unix timestamp to YYYY-MM-DD
func formatTimestamp(ts int64) string {
	t := time.Unix(ts, 0)
	return t.Format("2006-01-02")
}
