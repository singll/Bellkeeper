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

// N8NTriggerHandler triggers n8n workflow webhooks
type N8NTriggerHandler struct {
	BaseHandler
	name        string
	workflowURL string
	httpClient  *http.Client
}

// NewN8NTriggerHandler creates a new n8n trigger handler
func NewN8NTriggerHandler(name string, webhookURL string) *N8NTriggerHandler {
	return &N8NTriggerHandler{
		BaseHandler: BaseHandler{
			name:        name,
			description: fmt.Sprintf("触发 %s 工作流", name),
			usage:       "<参数>",
		},
		name:        name,
		workflowURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (h *N8NTriggerHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	// Prepare payload
	payload := map[string]interface{}{
		"room_id":   cmdCtx.RoomID,
		"sender":    cmdCtx.Sender,
		"event_id":  cmdCtx.EventID,
		"command":   cmdCtx.Command.Name,
		"args":      cmdCtx.Command.Args,
		"argv":      cmdCtx.Command.Argv,
		"timestamp": time.Now().Unix(),
	}

	// Send to n8n webhook
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.workflowURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 工作流调用失败: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &Response{
			Success: true,
			Message: fmt.Sprintf("✅ 已触发 %s", h.name),
		}, nil
	}

	return &Response{
		Success: false,
		Message: fmt.Sprintf("❌ 工作流返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody)),
	}, nil
}

// MemosTodoHandler handles Memos todo operations via n8n
type MemosTodoHandler struct {
	BaseHandler
	n8nWebhookURL string
	httpClient     *http.Client
}

// NewMemosTodoHandler creates a Memos todo handler
func NewMemosTodoHandler(webhookURL string) *MemosTodoHandler {
	return &MemosTodoHandler{
		BaseHandler: BaseHandler{
			name:        "memos",
			description: "Memos 待办管理",
			usage:       "<子命令> [参数]",
		},
		n8nWebhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (h *MemosTodoHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	// Parse subcommand
	if len(cmdCtx.Command.Argv) == 0 {
		return &Response{
			Success: true,
			Message: "**Memos 待办命令:**\n\n" +
				"• `!列表` - 查看所有待办\n" +
				"• `!新增 <内容>` - 添加新待办\n" +
				"• `!完成 <ID>` - 标记待办完成",
			IsHTML: true,
		}, nil
	}

	subCmd := cmdCtx.Command.Argv[0]
	args := ""
	if len(cmdCtx.Command.Argv) > 1 {
		args = cmdCtx.Command.Args[len(subCmd)+1:]
	}

	// Prepare payload for n8n
	payload := map[string]interface{}{
		"action":    subCmd,
		"args":      args,
		"room_id":   cmdCtx.RoomID,
		"sender":    cmdCtx.Sender,
		"event_id":  cmdCtx.EventID,
		"timestamp": time.Now().Unix(),
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", h.n8nWebhookURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 调用失败: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Try to parse response
		var result map[string]interface{}
		if json.Unmarshal(respBody, &result) == nil {
			if msg, ok := result["message"].(string); ok {
				return &Response{
					Success: true,
					Message: msg,
					IsHTML:  true,
				}, nil
			}
		}
		return &Response{
			Success: true,
			Message: fmt.Sprintf("✅ 执行成功"),
		}, nil
	}

	return &Response{
		Success: false,
		Message: fmt.Sprintf("❌ 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody)),
	}, nil
}

// QAHandler handles QA operations via n8n
type QAHandler struct {
	BaseHandler
	n8nWebhookURL string
	httpClient     *http.Client
}

// NewQAHandler creates a QA handler
func NewQAHandler(webhookURL string) *QAHandler {
	return &QAHandler{
		BaseHandler: BaseHandler{
			name:        "qa",
			description: "知识库问答",
			usage:       "<问题>",
		},
		n8nWebhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (h *QAHandler) Handle(ctx context.Context, cmdCtx *Context) (*Response, error) {
	question := cmdCtx.Command.Args
	if question == "" {
		return &Response{
			Success: true,
			Message: "请输入问题，例如: `!问 什么是 RAG？`",
			IsHTML:  false,
		}, nil
	}

	// Send to n8n
	payload := map[string]interface{}{
		"question": question,
		"room_id":  cmdCtx.RoomID,
		"sender":   cmdCtx.Sender,
		"event_id": cmdCtx.EventID,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", h.n8nWebhookURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Show "thinking" response
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return &Response{
			Success: false,
			Message: fmt.Sprintf("❌ 问答服务调用失败: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var result map[string]interface{}
		if json.Unmarshal(respBody, &result) == nil {
			if answer, ok := result["answer"].(string); ok {
				return &Response{
					Success: true,
					Message: fmt.Sprintf("**Q:** %s\n\n**A:** %s", question, answer),
					IsHTML:  true,
				}, nil
			}
		}
		return &Response{
			Success: true,
			Message: string(respBody),
		}, nil
	}

	return &Response{
		Success: false,
		Message: fmt.Sprintf("❌ 问答服务返回错误 (HTTP %d): %s", resp.StatusCode, string(respBody)),
	}, nil
}

// DirectMemosHandler handles Memos todo directly without n8n
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
			description: "Memos 待办管理（直接调用）",
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
			Message: "**待办命令:**\n\n" +
				"• `!待办 列表` - 查看所有待办\n" +
				"• `!待办 新增 <内容>` - 添加新待办\n" +
				"• `!待办 完成 <ID>` - 标记待办完成",
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
	case "新增", "add":
		return h.addTodo(ctx, args)
	case "完成", "done":
		return h.completeTodo(ctx, args)
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
			ID        int    `json:"id"`
			Content   string `json:"content"`
			CreatedAt string `json:"createdTs"`
			Done      bool   `json:"done"`
			Tags      []struct {
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

	// Filter todos (memos with "todo" tag or from specific resource)
	var todos []string
	for _, m := range result.Memos {
		// Skip done memos
		if m.Done {
			continue
		}
		// Check if it has a todo-related tag or content pattern
		isTodo := false
		for _, t := range m.Tags {
			if t.Name == "todo" || t.Name == "待办" || t.Name == "任务" {
				isTodo = true
				break
			}
		}
		// Also include memos from specific creator or with checkboxes
		if !isTodo && (len(m.Tags) == 0) {
			// Show all memos without tags as potential todos
			isTodo = true
		}
		if isTodo {
			todos = append(todos, fmt.Sprintf("• `[%d]` %s", m.ID, m.Content))
		}
	}

	if len(todos) == 0 {
		return &Response{
			Success: true,
			Message: "📋 待办列表\n\n暂无待办事项",
			IsHTML:  true,
		}, nil
	}

	msg := "📋 待办列表\n\n"
	for _, t := range todos {
		msg += t + "\n"
	}
	msg += "\n💡 使用 `!待办 完成 <ID>` 标记完成"

	return &Response{
		Success: true,
		Message: msg,
		IsHTML:  true,
	}, nil
}

func (h *DirectMemosHandler) addTodo(ctx context.Context, content string) (*Response, error) {
	if content == "" {
		return &Response{
			Success: false,
			Message: "请输入待办内容，例如: `!待办 新增 完成报告`",
		}, nil
	}

	if h.apiURL == "" || h.apiToken == "" {
		return &Response{
			Success: false,
			Message: "Memos API 未配置，请联系管理员",
		}, nil
	}

	url := h.apiURL + "/api/v1/memos"
	payload := map[string]interface{}{
		"content": content,
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
		return &Response{
			Success: true,
			Message: fmt.Sprintf("✅ 已添加待办 [#%d]: %s", result.Memo.ID, content),
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
		Message: fmt.Sprintf("✅ 已完成待办 #%d", id),
	}, nil
}
