package llmgateway

import (
	"fmt"
	"os"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/crypto"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// ChannelCredentialView is the API-safe projection of a stored credential. The
// encrypted CredentialJSON is never exposed; CredentialPreview reveals only the
// first/last few characters so operators can recognize which key is stored.
type ChannelCredentialView struct {
	model.LLMChannelCredential
	CredentialPreview string `json:"credential_preview"`
	EnvVarResolved    bool   `json:"env_var_resolved"`
}

// maskSecret renders a credential preview that reveals only the first and last
// few characters — never the full secret nor its exact length.
func maskSecret(s string) string {
	n := len(s)
	if n == 0 {
		return ""
	}
	if n <= 8 {
		return "****"
	}
	return s[:4] + "..." + s[n-4:]
}

// toCredentialView decrypts the stored credential to build a masked preview and
// strips the ciphertext from the returned copy.
func toCredentialView(c *model.LLMChannelCredential) ChannelCredentialView {
	v := ChannelCredentialView{LLMChannelCredential: *c}
	v.CredentialJSON = ""
	switch c.Source {
	case "env":
		if c.EnvVarName != "" {
			resolved := os.Getenv(c.EnvVarName)
			v.EnvVarResolved = resolved != ""
			if resolved != "" {
				v.CredentialPreview = "$" + c.EnvVarName
			} else {
				v.CredentialPreview = "$" + c.EnvVarName + " (未解析)"
			}
		}
	case "direct":
		if plain, err := crypto.Decrypt(c.CredentialJSON); err == nil {
			v.CredentialPreview = maskSecret(plain)
		} else {
			v.CredentialPreview = "<undecryptable>"
		}
	default:
		if c.CredentialJSON != "" {
			if plain, err := crypto.Decrypt(c.CredentialJSON); err == nil {
				v.CredentialPreview = maskSecret(plain)
			} else {
				v.CredentialPreview = "<undecryptable>"
			}
		}
	}
	return v
}

func resolveCredentialForChannel(credentialRepo *repository.LLMChannelCredentialRepository, channelID uint, purpose string) (string, error) {
	if credentialRepo == nil {
		return "", fmt.Errorf("credential repository not initialized")
	}
	creds, err := credentialRepo.ListByChannel(channelID)
	if err != nil {
		return "", fmt.Errorf("list credentials for channel %d: %w", channelID, err)
	}
	var active []model.LLMChannelCredential
	for _, c := range creds {
		if c.Purpose == purpose && c.Status == "active" {
			active = append(active, c)
		}
	}
	for _, c := range active {
		switch c.Source {
		case "env":
			if c.EnvVarName == "" {
				continue
			}
			if v := os.Getenv(c.EnvVarName); v != "" {
				return v, nil
			}
		case "direct":
			plain, err := credentialRepo.GetDecrypted(c.ID)
			if err != nil {
				middleware.GetLogger().Warn("credential decrypt failed, trying next",
					zap.Uint("credential_id", c.ID), zap.Uint("channel_id", channelID), zap.Error(err))
				continue
			}
			if plain != "" {
				return plain, nil
			}
		}
	}
	if len(active) > 0 {
		middleware.GetLogger().Warn("no usable credential found despite active records",
			zap.Uint("channel_id", channelID), zap.String("purpose", purpose), zap.Int("active_count", len(active)))
	}
	return "", nil
}

// ChannelBalanceHistory returns recent balance snapshots for a channel, newest first.
func (s *LLMProxyService) ChannelBalanceHistory(name string, limit int) ([]model.LLMChannelBalanceSnapshot, error) {
	if s.balanceSnapshotRepo == nil {
		return nil, fmt.Errorf("balance snapshot repository not initialized")
	}
	if name == "" {
		return nil, fmt.Errorf("channel name is required")
	}
	return s.balanceSnapshotRepo.ListByChannelName(name, limit)
}
