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
		httpClient: httpclient.NewClient(60 * time.Second),
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
