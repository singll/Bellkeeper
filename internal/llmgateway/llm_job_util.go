package llmgateway

import (
	"crypto/sha256"
	"encoding/hex"
)

func LLMJobIdempotencyKey(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return "llm:" + hex.EncodeToString(h.Sum(nil))
}
