package converter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const AnthropicVersion = "2023-06-01"

// ConvertOpenAIToAnthropic converts an OpenAI chat.completions request body
// to an Anthropic /v1/messages request body.
func ConvertOpenAIToAnthropic(body []byte) ([]byte, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse openai request: %w", err)
	}

	// Extract system messages
	var systemParts []string
	var messages []interface{}
	if raw, ok := req["messages"]; ok {
		if msgArr, ok := raw.([]interface{}); ok {
			for _, m := range msgArr {
				if msg, ok := m.(map[string]interface{}); ok {
					role, _ := msg["role"].(string)
					if role == "system" {
						content := extractTextContent(msg["content"])
						systemParts = append(systemParts, content)
						continue
					}
					// Convert tool_calls in assistant messages
					if role == "assistant" {
						messages = append(messages, convertAssistantMsg(msg))
						continue
					}
					// Convert tool result messages
					if role == "tool" {
						messages = append(messages, convertToolResultMsg(msg))
						continue
					}
					messages = append(messages, msg)
				}
			}
		}
	}

	// Build Anthropic request
	anthropic := map[string]interface{}{
		"messages": messages,
	}

	if len(systemParts) > 0 {
		anthropic["system"] = strings.Join(systemParts, "\n\n")
	}

	if mt, ok := req["max_tokens"]; ok {
		anthropic["max_tokens"] = mt
	} else {
		anthropic["max_tokens"] = 8192
	}

	// Direct pass-through fields
	for _, field := range []string{"model", "stream", "temperature", "top_p", "metadata"} {
		if v, ok := req[field]; ok {
			anthropic[field] = v
		}
	}

	// stop -> stop_sequences
	if stop, ok := req["stop"]; ok {
		anthropic["stop_sequences"] = normalizeStopSequences(stop)
	}

	// Convert functions/tools
	if tools := convertTools(req); tools != nil {
		anthropic["tools"] = tools
		if tc, ok := req["tool_choice"]; ok {
			anthropic["tool_choice"] = convertToolChoice(tc)
		}
	}

	return json.Marshal(anthropic)
}

// ConvertAnthropicToOpenAI converts an Anthropic /v1/messages response
// to an OpenAI chat.completions response.
func ConvertAnthropicToOpenAI(body []byte) ([]byte, error) {
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse anthropic response: %w", err)
	}

	// Check for Anthropic error response
	if resp["type"] == "error" {
		return ConvertAnthropicErrorToOpenAI(body)
	}

	msgID, _ := resp["id"].(string)
	model, _ := resp["model"].(string)

	// Build OpenAI response
	openai := map[string]interface{}{
		"id":      "chatcmpl-" + strings.TrimPrefix(msgID, "msg_"),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
	}

	// Convert content blocks to message
	content, _ := resp["content"].([]interface{})
	message := map[string]interface{}{
		"role": "assistant",
	}

	var textParts []string
	var toolCalls []interface{}

	for _, block := range content {
		if b, ok := block.(map[string]interface{}); ok {
			blockType, _ := b["type"].(string)
			switch blockType {
			case "text":
				if t, ok := b["text"].(string); ok {
					textParts = append(textParts, t)
				}
			case "tool_use":
				tc := map[string]interface{}{
					"id":   b["id"],
					"type": "function",
					"function": map[string]interface{}{
						"name":      b["name"],
						"arguments": serializeInput(b["input"]),
					},
				}
				toolCalls = append(toolCalls, tc)
			}
		}
	}

	if len(textParts) > 0 {
		message["content"] = strings.Join(textParts, "")
	} else if len(toolCalls) == 0 {
		message["content"] = nil
	}

	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	// stop_reason mapping
	finishReason := "stop"
	if sr, ok := resp["stop_reason"].(string); ok {
		switch sr {
		case "end_turn", "stop_sequence":
			finishReason = "stop"
		case "max_tokens":
			finishReason = "length"
		case "tool_use":
			finishReason = "tool_calls"
		default:
			finishReason = sr
		}
	}

	openai["choices"] = []interface{}{
		map[string]interface{}{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		},
	}

	// usage mapping
	openai["usage"] = convertAnthropicUsage(resp["usage"])

	return json.Marshal(openai)
}

// ConvertAnthropicErrorToOpenAI converts an Anthropic error response
// to an OpenAI-compatible error format.
func ConvertAnthropicErrorToOpenAI(body []byte) ([]byte, error) {
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return body, nil
	}

	errObj, _ := resp["error"].(map[string]interface{})
	errType := "api_error"
	errMsg := "unknown error"
	if errObj != nil {
		if t, ok := errObj["type"].(string); ok {
			errType = t
		}
		if m, ok := errObj["message"].(string); ok {
			errMsg = m
		}
	}

	openaiErr := map[string]interface{}{
		"error": map[string]interface{}{
			"message": errMsg,
			"type":    errType,
			"code":    nil,
		},
	}

	return json.Marshal(openaiErr)
}

// AnthropicSSEConverter converts Anthropic SSE events to OpenAI SSE format.
// It is stateful: it remembers message ID and model from message_start.
type AnthropicSSEConverter struct {
	msgID    string
	model    string
	created  int64
	toolIdx  int
}

// NewAnthropicSSEConverter creates a new converter.
func NewAnthropicSSEConverter() *AnthropicSSEConverter {
	return &AnthropicSSEConverter{
		created: time.Now().Unix(),
	}
}

// ConvertEvent converts a single Anthropic SSE event to OpenAI SSE data lines.
// Returns empty string for events with no OpenAI equivalent.
// Returns "data: [DONE]\n\n" when the stream ends.
func (c *AnthropicSSEConverter) ConvertEvent(eventType, data string) string {
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return ""
	}

	switch eventType {
	case "message_start":
		return c.handleMessageStart(event)
	case "content_block_start":
		return c.handleContentBlockStart(event)
	case "content_block_delta":
		return c.handleContentBlockDelta(event)
	case "content_block_stop":
		return ""
	case "message_delta":
		return c.handleMessageDelta(event)
	case "message_stop":
		return "data: [DONE]\n\n"
	case "ping":
		return ""
	default:
		return ""
	}
}

func (c *AnthropicSSEConverter) handleMessageStart(event map[string]interface{}) string {
	msg, _ := event["message"].(map[string]interface{})
	if msg != nil {
		if id, ok := msg["id"].(string); ok {
			c.msgID = "chatcmpl-" + strings.TrimPrefix(id, "msg_")
		}
		if m, ok := msg["model"].(string); ok {
			c.model = m
		}
	}

	// Emit first chunk with role
	chunk := map[string]interface{}{
		"id":      c.msgID,
		"object":  "chat.completion.chunk",
		"created": c.created,
		"model":   c.model,
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"role": "assistant",
				},
				"finish_reason": nil,
			},
		},
	}
	return formatSSEChunk(chunk)
}

func (c *AnthropicSSEConverter) handleContentBlockStart(event map[string]interface{}) string {
	block, _ := event["content_block"].(map[string]interface{})
	if block == nil {
		return ""
	}

	blockType, _ := block["type"].(string)
	if blockType == "tool_use" {
		idx := c.toolIdx
		c.toolIdx++

		chunk := map[string]interface{}{
			"id":      c.msgID,
			"object":  "chat.completion.chunk",
			"created": c.created,
			"model":   c.model,
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"delta": map[string]interface{}{
						"tool_calls": []interface{}{
							map[string]interface{}{
								"index": idx,
								"id":    block["id"],
								"type":  "function",
								"function": map[string]interface{}{
									"name":      block["name"],
									"arguments": "",
								},
							},
						},
					},
					"finish_reason": nil,
				},
			},
		}
		return formatSSEChunk(chunk)
	}

	return ""
}

func (c *AnthropicSSEConverter) handleContentBlockDelta(event map[string]interface{}) string {
	delta, _ := event["delta"].(map[string]interface{})
	if delta == nil {
		return ""
	}

	deltaType, _ := delta["type"].(string)

	switch deltaType {
	case "text_delta":
		text, _ := delta["text"].(string)
		chunk := map[string]interface{}{
			"id":      c.msgID,
			"object":  "chat.completion.chunk",
			"created": c.created,
			"model":   c.model,
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"delta": map[string]interface{}{
						"content": text,
					},
					"finish_reason": nil,
				},
			},
		}
		return formatSSEChunk(chunk)

	case "input_json_delta":
		partialJSON, _ := delta["partial_json"].(string)
		idx := c.toolIdx
		if idx > 0 {
			idx--
		}
		chunk := map[string]interface{}{
			"id":      c.msgID,
			"object":  "chat.completion.chunk",
			"created": c.created,
			"model":   c.model,
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"delta": map[string]interface{}{
						"tool_calls": []interface{}{
							map[string]interface{}{
								"index": idx,
								"function": map[string]interface{}{
									"arguments": partialJSON,
								},
							},
						},
					},
					"finish_reason": nil,
				},
			},
		}
		return formatSSEChunk(chunk)
	}

	return ""
}

func (c *AnthropicSSEConverter) handleMessageDelta(event map[string]interface{}) string {
	delta, _ := event["delta"].(map[string]interface{})
	if delta == nil {
		return ""
	}

	finishReason := ""
	if sr, ok := delta["stop_reason"].(string); ok {
		switch sr {
		case "end_turn", "stop_sequence":
			finishReason = "stop"
		case "max_tokens":
			finishReason = "length"
		case "tool_use":
			finishReason = "tool_calls"
		default:
			finishReason = sr
		}
	}

	chunk := map[string]interface{}{
		"id":      c.msgID,
		"object":  "chat.completion.chunk",
		"created": c.created,
		"model":   c.model,
		"choices": []interface{}{
			map[string]interface{}{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": finishReason,
			},
		},
	}
	return formatSSEChunk(chunk)
}

// --- Helper functions ---

func formatSSEChunk(chunk map[string]interface{}) string {
	data, err := json.Marshal(chunk)
	if err != nil {
		return ""
	}
	return "data: " + string(data) + "\n\n"
}

func extractTextContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func convertAssistantMsg(msg map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"role": "assistant",
	}

	// Extract text content
	if content, ok := msg["content"]; ok {
		result["content"] = extractTextContent(content)
	}

	// Convert tool_calls
	if rawTC, ok := msg["tool_calls"].([]interface{}); ok && len(rawTC) > 0 {
		var contentBlocks []interface{}
		// Add text content block if present
		if text, ok := result["content"].(string); ok && text != "" {
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type": "text",
				"text": text,
			})
		}
		// Add tool_use blocks
		for _, tc := range rawTC {
			if t, ok := tc.(map[string]interface{}); ok {
				fn, _ := t["function"].(map[string]interface{})
				name, _ := fn["name"].(string)
				args, _ := fn["arguments"].(string)
				block := map[string]interface{}{
					"type":  "tool_use",
					"id":    t["id"],
					"name":  name,
					"input": parseJSONArgs(args),
				}
				contentBlocks = append(contentBlocks, block)
			}
		}
		result["content"] = contentBlocks
		delete(result, "tool_calls")
	}

	return result
}

func convertToolResultMsg(msg map[string]interface{}) map[string]interface{} {
	toolCallID, _ := msg["tool_call_id"].(string)
	content := extractTextContent(msg["content"])

	return map[string]interface{}{
		"role": "user",
		"content": []interface{}{
			map[string]interface{}{
				"type":       "tool_result",
				"tool_use_id": toolCallID,
				"content":    content,
			},
		},
	}
}

func convertTools(req map[string]interface{}) []interface{} {
	var tools []interface{}

	if rawTools, ok := req["tools"].([]interface{}); ok {
		for _, t := range rawTools {
			if tool, ok := t.(map[string]interface{}); ok {
				tType, _ := tool["type"].(string)
				if tType == "function" {
					fn, _ := tool["function"].(map[string]interface{})
					if fn != nil {
						anthropicTool := map[string]interface{}{
							"name":         fn["name"],
							"description":  fn["description"],
							"input_schema": fn["parameters"],
						}
						tools = append(tools, anthropicTool)
					}
				}
			}
		}
	}

	if rawFns, ok := req["functions"].([]interface{}); ok {
		for _, f := range rawFns {
			if fn, ok := f.(map[string]interface{}); ok {
				anthropicTool := map[string]interface{}{
					"name":         fn["name"],
					"description":  fn["description"],
					"input_schema": fn["parameters"],
				}
				tools = append(tools, anthropicTool)
			}
		}
	}

	return tools
}

func convertToolChoice(tc interface{}) interface{} {
	switch v := tc.(type) {
	case string:
		switch v {
		case "auto":
			return map[string]interface{}{"type": "auto"}
		case "none":
			return map[string]interface{}{"type": "none"}
		case "required":
			return map[string]interface{}{"type": "any"}
		default:
			return map[string]interface{}{"type": "auto"}
		}
	case map[string]interface{}:
		if v["type"] == "function" {
			if fn, ok := v["function"].(map[string]interface{}); ok {
				if name, ok := fn["name"].(string); ok {
					return map[string]interface{}{
						"type": "tool",
						"name": name,
					}
				}
			}
		}
		return map[string]interface{}{"type": "auto"}
	default:
		return map[string]interface{}{"type": "auto"}
	}
}

func normalizeStopSequences(stop interface{}) []string {
	switch v := stop.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var result []string
		for _, s := range v {
			if str, ok := s.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}

func convertAnthropicUsage(usage interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"prompt_tokens":     0,
		"completion_tokens": 0,
		"total_tokens":      0,
	}

	if u, ok := usage.(map[string]interface{}); ok {
		input, _ := u["input_tokens"].(float64)
		output, _ := u["output_tokens"].(float64)
		result["prompt_tokens"] = int(input)
		result["completion_tokens"] = int(output)
		result["total_tokens"] = int(input + output)
	}

	return result
}

func serializeInput(input interface{}) string {
	if input == nil {
		return "{}"
	}
	data, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func parseJSONArgs(args string) interface{} {
	var result interface{}
	if err := json.Unmarshal([]byte(args), &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}
