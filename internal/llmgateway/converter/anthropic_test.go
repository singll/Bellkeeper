package converter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertOpenAIToAnthropic_SystemMessages(t *testing.T) {
	input := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []interface{}{
			map[string]interface{}{"role": "system", "content": "You are helpful."},
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	}
	body, _ := json.Marshal(input)

	result, err := ConvertOpenAIToAnthropic(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	assert.Equal(t, "You are helpful.", out["system"])

	msgs, ok := out["messages"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, msgs, 1)

	firstMsg, ok := msgs[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "user", firstMsg["role"])
}

func TestConvertOpenAIToAnthropic_DefaultMaxTokens(t *testing.T) {
	input := map[string]interface{}{
		"model": "claude-3-5-sonnet-20241022",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hi"},
		},
	}
	body, _ := json.Marshal(input)

	result, err := ConvertOpenAIToAnthropic(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	assert.Equal(t, float64(8192), out["max_tokens"])
}

func TestConvertOpenAIToAnthropic_ExplicitMaxTokens(t *testing.T) {
	input := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 2048,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hi"},
		},
	}
	body, _ := json.Marshal(input)

	result, err := ConvertOpenAIToAnthropic(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	assert.Equal(t, float64(2048), out["max_tokens"])
}

func TestConvertOpenAIToAnthropic_ToolConversion(t *testing.T) {
	input := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "What's the weather?"},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "get_weather",
					"description": "Get weather for a location",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(input)

	result, err := ConvertOpenAIToAnthropic(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	tools, ok := out["tools"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, tools, 1)

	tool, ok := tools[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "get_weather", tool["name"])
	assert.Equal(t, "Get weather for a location", tool["description"])
	assert.NotNil(t, tool["input_schema"])
}

func TestConvertOpenAIToAnthropic_ToolChoiceConversion(t *testing.T) {
	tests := []struct {
		name     string
		choice   interface{}
		expected interface{}
	}{
		{
			name:     "auto",
			choice:   "auto",
			expected: map[string]interface{}{"type": "auto"},
		},
		{
			name:     "none",
			choice:   "none",
			expected: map[string]interface{}{"type": "none"},
		},
		{
			name:     "required maps to any",
			choice:   "required",
			expected: map[string]interface{}{"type": "any"},
		},
		{
			name: "function maps to named tool",
			choice: map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "get_weather",
				},
			},
			expected: map[string]interface{}{"type": "tool", "name": "get_weather"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToolChoice(tt.choice)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestConvertOpenAIToAnthropic_AssistantToolCalls(t *testing.T) {
	input := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Check weather"},
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   "call_123",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "get_weather",
							"arguments": `{"location":"Tokyo"}`,
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(input)

	result, err := ConvertOpenAIToAnthropic(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	msgs, ok := out["messages"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, msgs, 2)

	assistantMsg, ok := msgs[1].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "assistant", assistantMsg["role"])

	contentBlocks, ok := assistantMsg["content"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, contentBlocks, 1)

	toolUseBlock, ok := contentBlocks[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "tool_use", toolUseBlock["type"])
	assert.Equal(t, "call_123", toolUseBlock["id"])
	assert.Equal(t, "get_weather", toolUseBlock["name"])
}

func TestConvertOpenAIToAnthropic_ToolResultMessage(t *testing.T) {
	input := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Check weather"},
			map[string]interface{}{
				"role":    "tool",
				"content": "Sunny, 25°C",
				"tool_call_id": "call_123",
			},
		},
	}
	body, _ := json.Marshal(input)

	result, err := ConvertOpenAIToAnthropic(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	msgs, ok := out["messages"].([]interface{})
	assert.True(t, ok)

	toolResultMsg, ok := msgs[1].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "user", toolResultMsg["role"])

	content, ok := toolResultMsg["content"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, content, 1)

	block, ok := content[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "tool_result", block["type"])
	assert.Equal(t, "call_123", block["tool_use_id"])
	assert.Equal(t, "Sunny, 25°C", block["content"])
}

func TestConvertOpenAIToAnthropic_StopSequences(t *testing.T) {
	tests := []struct {
		name     string
		stop     interface{}
		expected []string
	}{
		{"string", "STOP", []string{"STOP"}},
		{"array", []interface{}{"STOP", "END"}, []string{"STOP", "END"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeStopSequences(tt.stop)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestConvertOpenAIToAnthropic_InvalidJSON(t *testing.T) {
	_, err := ConvertOpenAIToAnthropic([]byte("not json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse openai request")
}

func TestConvertAnthropicToOpenAI_TextResponse(t *testing.T) {
	anthropicResp := map[string]interface{}{
		"id":    "msg_abc123",
		"model": "claude-3-5-sonnet-20241022",
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "Hello!"},
		},
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{
			"input_tokens":  float64(10),
			"output_tokens": float64(5),
		},
	}
	body, _ := json.Marshal(anthropicResp)

	result, err := ConvertAnthropicToOpenAI(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	assert.Equal(t, "chatcmpl-abc123", out["id"])
	assert.Equal(t, "chat.completion", out["object"])
	assert.Equal(t, "claude-3-5-sonnet-20241022", out["model"])

	choices, ok := out["choices"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, choices, 1)

	choice, ok := choices[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, float64(0), choice["index"])
	assert.Equal(t, "stop", choice["finish_reason"])

	msg, ok := choice["message"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "assistant", msg["role"])
	assert.Equal(t, "Hello!", msg["content"])
}

func TestConvertAnthropicToOpenAI_ToolUseResponse(t *testing.T) {
	anthropicResp := map[string]interface{}{
		"id":    "msg_xyz789",
		"model": "claude-3-5-sonnet-20241022",
		"content": []interface{}{
			map[string]interface{}{
				"type":  "tool_use",
				"id":    "toolu_123",
				"name":  "get_weather",
				"input": map[string]interface{}{"location": "Tokyo"},
			},
		},
		"stop_reason": "tool_use",
		"usage": map[string]interface{}{
			"input_tokens":  float64(20),
			"output_tokens": float64(15),
		},
	}
	body, _ := json.Marshal(anthropicResp)

	result, err := ConvertAnthropicToOpenAI(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	choices, ok := out["choices"].([]interface{})
	assert.True(t, ok)
	choice := choices[0].(map[string]interface{})

	assert.Equal(t, "tool_calls", choice["finish_reason"])

	msg := choice["message"].(map[string]interface{})
	toolCalls, ok := msg["tool_calls"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, toolCalls, 1)

	tc := toolCalls[0].(map[string]interface{})
	assert.Equal(t, "toolu_123", tc["id"])
	assert.Equal(t, "function", tc["type"])

	fn := tc["function"].(map[string]interface{})
	assert.Equal(t, "get_weather", fn["name"])
	assert.Contains(t, fn["arguments"].(string), "Tokyo")
}

func TestConvertAnthropicToOpenAI_StopReasonMapping(t *testing.T) {
	tests := []struct {
		anthropicReason string
		openAIReason    string
	}{
		{"end_turn", "stop"},
		{"stop_sequence", "stop"},
		{"max_tokens", "length"},
		{"tool_use", "tool_calls"},
	}

	for _, tt := range tests {
		t.Run(tt.anthropicReason, func(t *testing.T) {
			resp := map[string]interface{}{
				"id":    "msg_test",
				"model": "claude-3-5-sonnet-20241022",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "hi"},
				},
				"stop_reason": tt.anthropicReason,
				"usage": map[string]interface{}{
					"input_tokens":  float64(1),
					"output_tokens": float64(1),
				},
			}
			body, _ := json.Marshal(resp)

			result, err := ConvertAnthropicToOpenAI(body)
			assert.NoError(t, err)

			var out map[string]interface{}
			assert.NoError(t, json.Unmarshal(result, &out))

			choices := out["choices"].([]interface{})
			choice := choices[0].(map[string]interface{})
			assert.Equal(t, tt.openAIReason, choice["finish_reason"])
		})
	}
}

func TestConvertAnthropicToOpenAI_UsageMapping(t *testing.T) {
	anthropicResp := map[string]interface{}{
		"id":    "msg_usage",
		"model": "claude-3-5-sonnet-20241022",
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "ok"},
		},
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{
			"input_tokens":  float64(100),
			"output_tokens": float64(50),
		},
	}
	body, _ := json.Marshal(anthropicResp)

	result, err := ConvertAnthropicToOpenAI(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	usage, ok := out["usage"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, float64(100), usage["prompt_tokens"])
	assert.Equal(t, float64(50), usage["completion_tokens"])
	assert.Equal(t, float64(150), usage["total_tokens"])
}

func TestConvertAnthropicErrorToOpenAI(t *testing.T) {
	anthropicErr := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "rate_limit_error",
			"message": "Rate limit exceeded",
		},
	}
	body, _ := json.Marshal(anthropicErr)

	result, err := ConvertAnthropicErrorToOpenAI(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	errObj, ok := out["error"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "Rate limit exceeded", errObj["message"])
	assert.Equal(t, "rate_limit_error", errObj["type"])
}

func TestConvertAnthropicToOpenAI_ErrorResponseRedirect(t *testing.T) {
	anthropicErr := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "invalid_request_error",
			"message": "Bad request",
		},
	}
	body, _ := json.Marshal(anthropicErr)

	result, err := ConvertAnthropicToOpenAI(body)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	errObj, ok := out["error"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "Bad request", errObj["message"])
}

func TestConvertAnthropicToOpenAI_InvalidJSON(t *testing.T) {
	_, err := ConvertAnthropicToOpenAI([]byte("not json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse anthropic response")
}

func TestAnthropicSSEConverter_MessageStart(t *testing.T) {
	c := NewAnthropicSSEConverter()
	data := `{"message":{"id":"msg_001","model":"claude-3-5-sonnet-20241022"}}`

	output := c.ConvertEvent("message_start", data)
	assert.Contains(t, output, "data: ")
	assert.Contains(t, output, "chat.completion.chunk")
	assert.Contains(t, output, `"role":"assistant"`)
}

func TestAnthropicSSEConverter_TextDelta(t *testing.T) {
	c := NewAnthropicSSEConverter()
	c.msgID = "chatcmpl-001"
	c.model = "claude-3-5-sonnet-20241022"

	data := `{"delta":{"type":"text_delta","text":"Hello"}}`

	output := c.ConvertEvent("content_block_delta", data)
	assert.Contains(t, output, "chat.completion.chunk")
	assert.Contains(t, output, `"content":"Hello"`)
}

func TestAnthropicSSEConverter_ToolUseBlockStart(t *testing.T) {
	c := NewAnthropicSSEConverter()
	c.msgID = "chatcmpl-001"
	c.model = "claude-3-5-sonnet-20241022"

	data := `{"content_block":{"type":"tool_use","id":"toolu_abc","name":"get_weather"}}`

	output := c.ConvertEvent("content_block_start", data)
	assert.Contains(t, output, "chat.completion.chunk")
	assert.Contains(t, output, `"tool_calls"`)
	assert.Contains(t, output, `"name":"get_weather"`)
}

func TestAnthropicSSEConverter_InputJSONDelta(t *testing.T) {
	c := NewAnthropicSSEConverter()
	c.msgID = "chatcmpl-001"
	c.model = "claude-3-5-sonnet-20241022"
	c.toolIdx = 1

	data := `{"delta":{"type":"input_json_delta","partial_json":"{\"loc"}}`

	output := c.ConvertEvent("content_block_delta", data)
	assert.Contains(t, output, "chat.completion.chunk")
	assert.Contains(t, output, `"arguments":"`)
}

func TestAnthropicSSEConverter_MessageDeltaFinishReason(t *testing.T) {
	c := NewAnthropicSSEConverter()
	c.msgID = "chatcmpl-001"
	c.model = "claude-3-5-sonnet-20241022"

	tests := []struct {
		name           string
		data           string
		expectedReason string
	}{
		{"end_turn", `{"delta":{"stop_reason":"end_turn"}}`, "stop"},
		{"max_tokens", `{"delta":{"stop_reason":"max_tokens"}}`, "length"},
		{"tool_use", `{"delta":{"stop_reason":"tool_use"}}`, "tool_calls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := c.ConvertEvent("message_delta", tt.data)
			assert.Contains(t, output, "chat.completion.chunk")
			assert.Contains(t, output, `"finish_reason":"`+tt.expectedReason+`"`)
		})
	}
}

func TestAnthropicSSEConverter_MessageStop(t *testing.T) {
	c := NewAnthropicSSEConverter()

	output := c.ConvertEvent("message_stop", "{}")
	assert.Equal(t, "data: [DONE]\n\n", output)
}

func TestAnthropicSSEConverter_IgnoredEvents(t *testing.T) {
	c := NewAnthropicSSEConverter()

	assert.Empty(t, c.ConvertEvent("content_block_stop", "{}"))
	assert.Empty(t, c.ConvertEvent("ping", "{}"))
	assert.Empty(t, c.ConvertEvent("unknown_event", "{}"))
}

func TestAnthropicSSEConverter_InvalidData(t *testing.T) {
	c := NewAnthropicSSEConverter()
	assert.Empty(t, c.ConvertEvent("message_start", "not json"))
}

func TestAnthropicSSEConverter_FullStream(t *testing.T) {
	c := NewAnthropicSSEConverter()

	var collected []string

	events := []struct {
		eventType string
		data      string
	}{
		{"message_start", `{"message":{"id":"msg_full","model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":10}}}`},
		{"content_block_start", `{"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"delta":{"type":"text_delta","text":"Hello "}}`},
		{"content_block_delta", `{"delta":{"type":"text_delta","text":"World"}}`},
		{"content_block_stop", `{}`},
		{"message_delta", `{"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`},
		{"message_stop", `{}`},
	}

	for _, e := range events {
		output := c.ConvertEvent(e.eventType, e.data)
		if output != "" {
			collected = append(collected, output)
		}
	}

	assert.True(t, len(collected) >= 3, "should produce at least 3 output events")

	fullOutput := strings.Join(collected, "")
	assert.Contains(t, fullOutput, `"role":"assistant"`)
	assert.Contains(t, fullOutput, `"content":"Hello "`)
	assert.Contains(t, fullOutput, `"content":"World"`)
	assert.Contains(t, fullOutput, `"finish_reason":"stop"`)
	assert.Contains(t, fullOutput, "[DONE]")
}

func TestExtractTextContent(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"string", "hello", "hello"},
		{"content array", []interface{}{
			map[string]interface{}{"text": "hello "},
			map[string]interface{}{"text": "world"},
		}, "hello world"},
		{"other type", 42, "42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextContent(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseJSONArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"valid json", `{"key":"value"}`},
		{"invalid json returns empty map", "not json"},
		{"empty string returns empty map", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseJSONArgs(tt.input)
			assert.NotNil(t, got)
		})
	}
}

func TestSerializeInput(t *testing.T) {
	assert.Equal(t, "{}", serializeInput(nil))
	assert.Equal(t, `{"key":"val"}`, serializeInput(map[string]string{"key": "val"}))
}
