package converter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenAIToGemini_BasicConversion(t *testing.T) {
	input := map[string]interface{}{
		"model": "gemini-1.5-pro",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	}
	body, _ := json.Marshal(input)

	result, err := OpenAIToGemini(body)
	assert.NoError(t, err)

	var out GeminiRequest
	assert.NoError(t, json.Unmarshal(result, &out))

	assert.Len(t, out.Contents, 1)
	assert.Equal(t, "user", out.Contents[0].Role)
	assert.Len(t, out.Contents[0].Parts, 1)
	assert.Equal(t, "Hello", out.Contents[0].Parts[0].Text)
}

func TestOpenAIToGemini_SystemMessageBecomesUser(t *testing.T) {
	input := map[string]interface{}{
		"model": "gemini-1.5-pro",
		"messages": []interface{}{
			map[string]interface{}{"role": "system", "content": "You are helpful."},
			map[string]interface{}{"role": "user", "content": "Hi"},
		},
	}
	body, _ := json.Marshal(input)

	result, err := OpenAIToGemini(body)
	assert.NoError(t, err)

	var out GeminiRequest
	assert.NoError(t, json.Unmarshal(result, &out))

	assert.Len(t, out.Contents, 2)
	assert.Equal(t, "user", out.Contents[0].Role, "system message should become user role")
	assert.Equal(t, "You are helpful.", out.Contents[0].Parts[0].Text)
}

func TestOpenAIToGemini_GenerationConfig(t *testing.T) {
	temp := 0.7
	topP := 0.9
	input := map[string]interface{}{
		"model":       "gemini-1.5-pro",
		"temperature": temp,
		"max_tokens":  256,
		"top_p":       topP,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hi"},
		},
	}
	body, _ := json.Marshal(input)

	result, err := OpenAIToGemini(body)
	assert.NoError(t, err)

	var out GeminiRequest
	assert.NoError(t, json.Unmarshal(result, &out))

	assert.NotNil(t, out.GenerationConfig)
	assert.Equal(t, 0.7, out.GenerationConfig.Temperature)
	assert.Equal(t, 256, out.GenerationConfig.MaxOutputTokens)
	assert.Equal(t, 0.9, out.GenerationConfig.TopP)
}

func TestOpenAIToGemini_NoGenerationConfigWhenNotNeeded(t *testing.T) {
	input := map[string]interface{}{
		"model": "gemini-1.5-pro",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hi"},
		},
	}
	body, _ := json.Marshal(input)

	result, err := OpenAIToGemini(body)
	assert.NoError(t, err)

	var out GeminiRequest
	assert.NoError(t, json.Unmarshal(result, &out))

	assert.Nil(t, out.GenerationConfig)
}

func TestOpenAIToGemini_InvalidJSON(t *testing.T) {
	_, err := OpenAIToGemini([]byte("not json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse openai request")
}

func TestGeminiToOpenAI_BasicResponse(t *testing.T) {
	geminiResp := GeminiResponse{
		Candidates: []struct {
			Content      GeminiContent `json:"content"`
			FinishReason string        `json:"finishReason"`
		}{
			{
				Content:      GeminiContent{Role: "model", Parts: []GeminiPart{{Text: "Hello!"}}},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: &struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		}{PromptTokenCount: 10, CandidatesTokenCount: 5, TotalTokenCount: 15},
	}
	body, _ := json.Marshal(geminiResp)

	result, err := GeminiToOpenAI(body, "gemini-1.5-pro")
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	assert.Equal(t, "chat.completion", out["object"])
	assert.Equal(t, "gemini-1.5-pro", out["model"])

	choices, ok := out["choices"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, choices, 1)

	choice, ok := choices[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "stop", choice["finish_reason"])

	msg, ok := choice["message"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "assistant", msg["role"])
	assert.Equal(t, "Hello!", msg["content"])

	usage, ok := out["usage"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, float64(10), usage["prompt_tokens"])
	assert.Equal(t, float64(5), usage["completion_tokens"])
	assert.Equal(t, float64(15), usage["total_tokens"])
}

func TestGeminiToOpenAI_NoCandidates(t *testing.T) {
	geminiResp := GeminiResponse{}
	body, _ := json.Marshal(geminiResp)

	_, err := GeminiToOpenAI(body, "gemini-1.5-pro")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no candidates")
}

func TestGeminiToOpenAI_FinishReasonMapping(t *testing.T) {
	tests := []struct {
		name           string
		finishReason   string
		expectedReason string
	}{
		{"STOP maps to stop", "STOP", "stop"},
		{"MAX_TOKENS maps to length", "MAX_TOKENS", "length"},
		{"SAFETY maps to content_filter", "SAFETY", "content_filter"},
		{"RECITATION maps to content_filter", "RECITATION", "content_filter"},
		{"unknown maps to stop", "UNKNOWN", "stop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapFinishReason(tt.finishReason)
			assert.Equal(t, tt.expectedReason, got)
		})
	}
}

func TestGeminiToOpenAI_InvalidJSON(t *testing.T) {
	_, err := GeminiToOpenAI([]byte("not json"), "gemini-1.5-pro")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse gemini response")
}

func TestGeminiToOpenAI_NoUsageMetadata(t *testing.T) {
	geminiResp := GeminiResponse{
		Candidates: []struct {
			Content      GeminiContent `json:"content"`
			FinishReason string        `json:"finishReason"`
		}{
			{
				Content:      GeminiContent{Role: "model", Parts: []GeminiPart{{Text: "ok"}}},
				FinishReason: "STOP",
			},
		},
	}
	body, _ := json.Marshal(geminiResp)

	result, err := GeminiToOpenAI(body, "gemini-1.5-pro")
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &out))

	_, hasUsage := out["usage"]
	assert.False(t, hasUsage, "should not have usage when UsageMetadata is nil")
}

func TestOpenAIToGemini_RoundTrip(t *testing.T) {
	openaiReq := map[string]interface{}{
		"model": "gemini-1.5-pro",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "What is 2+2?"},
			map[string]interface{}{"role": "assistant", "content": "4"},
			map[string]interface{}{"role": "user", "content": "And 3+3?"},
		},
		"temperature": 0.5,
		"max_tokens":  100,
	}
	body, _ := json.Marshal(openaiReq)

	geminiBody, err := OpenAIToGemini(body)
	assert.NoError(t, err)

	var geminiReq GeminiRequest
	assert.NoError(t, json.Unmarshal(geminiBody, &geminiReq))

	assert.Len(t, geminiReq.Contents, 3)
	assert.Equal(t, "user", geminiReq.Contents[0].Role)
	assert.Equal(t, "assistant", geminiReq.Contents[1].Role)
	assert.Equal(t, "user", geminiReq.Contents[2].Role)
	assert.NotNil(t, geminiReq.GenerationConfig)
	assert.Equal(t, 0.5, geminiReq.GenerationConfig.Temperature)
	assert.Equal(t, 100, geminiReq.GenerationConfig.MaxOutputTokens)
}
