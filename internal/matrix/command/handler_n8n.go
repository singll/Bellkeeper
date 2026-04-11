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
		httpClient: httpclient.NewClient(30 * time.Second),
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
