package converter

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GeminiRequest converts an OpenAI chat/completions request to Gemini generateContent format.
type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
	GenerationConfig *GeminiGenerationConfig `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Role  string `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiInlineData struct {
	Source string `json:"source,omitempty"`
}

type GeminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *GeminiInlineData `json:"inlineData,omitempty"`
}

type GeminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
}

// GeminiResponse is the Gemini API response format.
type GeminiResponse struct {
	Candidates []struct {
		Content GeminiContent `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// openAIMessage represents a message from an OpenAI-compatible request,
// supporting both string content and content part arrays (multi-modal).
type openAIMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// contentPart is a single element of an OpenAI content array.
type contentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

// OpenAIToGemini converts an OpenAI chat/completions request body to Gemini format.
func OpenAIToGemini(openAIBody []byte) ([]byte, error) {
	var openaiReq struct {
		Model    string          `json:"model"`
		Messages []openAIMessage `json:"messages"`
		Temperature     *float64 `json:"temperature,omitempty"`
		MaxTokens       int      `json:"max_tokens,omitempty"`
		TopP            *float64 `json:"top_p,omitempty"`
		Stream          bool     `json:"stream,omitempty"`
	}
	if err := json.Unmarshal(openAIBody, &openaiReq); err != nil {
		return nil, fmt.Errorf("parse openai request: %w", err)
	}

	geminiReq := GeminiRequest{
		Contents: make([]GeminiContent, 0, len(openaiReq.Messages)),
	}

	for _, msg := range openaiReq.Messages {
		role := msg.Role
		if role == "system" {
			role = "user"
		}
		parts := parseOpenAIContent(msg.Content)
		geminiReq.Contents = append(geminiReq.Contents, GeminiContent{
			Role:  role,
			Parts: parts,
		})
	}

	if openaiReq.Temperature != nil || openaiReq.MaxTokens > 0 || openaiReq.TopP != nil {
		cfg := &GeminiGenerationConfig{}
		if openaiReq.Temperature != nil {
			cfg.Temperature = *openaiReq.Temperature
		}
		if openaiReq.MaxTokens > 0 {
			cfg.MaxOutputTokens = openaiReq.MaxTokens
		}
		if openaiReq.TopP != nil {
			cfg.TopP = *openaiReq.TopP
		}
		geminiReq.GenerationConfig = cfg
	}

	return json.Marshal(geminiReq)
}

// GeminiToOpenAI converts a Gemini response to OpenAI chat/completions format.
func GeminiToOpenAI(geminiBody []byte, model string) ([]byte, error) {
	var geminiResp GeminiResponse
	if err := json.Unmarshal(geminiBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("parse gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in gemini response")
	}

	candidate := geminiResp.Candidates[0]
	content := ""
	if len(candidate.Content.Parts) > 0 {
		content = candidate.Content.Parts[0].Text
	}

	openaiResp := map[string]interface{}{
		"id":      "gemini-" + generateID(),
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]interface{}{"role": "assistant", "content": content},
				"finish_reason": mapFinishReason(candidate.FinishReason),
			},
		},
	}

	if geminiResp.UsageMetadata != nil {
		openaiResp["usage"] = map[string]int{
			"prompt_tokens":     geminiResp.UsageMetadata.PromptTokenCount,
			"completion_tokens": geminiResp.UsageMetadata.CandidatesTokenCount,
			"total_tokens":      geminiResp.UsageMetadata.TotalTokenCount,
		}
	}

	return json.Marshal(openaiResp)
}

func mapFinishReason(reason string) string {
	switch strings.ToUpper(reason) {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION":
		return "content_filter"
	default:
		return "stop"
	}
}

// parseOpenAIContent parses an OpenAI content field that may be a plain string or
// an array of ContentPart (for multi-modal messages). Returns Gemini parts.
func parseOpenAIContent(raw json.RawMessage) []GeminiPart {
	if len(raw) == 0 {
		return []GeminiPart{{Text: ""}}
	}
	// Try array first (multi-modal: [{"type":"text","text":"..."},{"type":"image_url","image_url":{"url":"..."}}])
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err == nil && len(parts) > 0 {
		geminiParts := make([]GeminiPart, 0, len(parts))
		for _, p := range parts {
			switch p.Type {
			case "text":
				geminiParts = append(geminiParts, GeminiPart{Text: p.Text})
			case "image_url":
				if p.ImageURL != nil {
					geminiParts = append(geminiParts, GeminiPart{InlineData: &GeminiInlineData{Source: p.ImageURL.URL}})
				}
			}
		}
		if len(geminiParts) > 0 {
			return geminiParts
		}
	}
	// Fall back to plain string
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []GeminiPart{{Text: text}}
	}
	return []GeminiPart{{Text: string(raw)}}
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return hex.EncodeToString(b[:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:])
}
