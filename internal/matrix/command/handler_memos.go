package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/singll/bellkeeper/internal/pkg/httpclient"
)

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
		httpClient: httpclient.NewClient(30 * time.Second),
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
