package service

import (
	"fmt"
	"os"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/crypto"
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

// reloadAfterCredentialChange reloads runtime config so a credential mutation takes
// effect immediately — a channel's API key is resolved into Channel.Config only at
// load time, so without this a newly added/edited key would not be used until the
// next restart or explicit reload. The credential row is already persisted, so a
// reload failure is logged rather than propagated (Channel/Group CRUD reload alike).
func (s *LLMProxyService) reloadAfterCredentialChange(op string) {
	if err := s.Reload(); err != nil {
		middleware.GetLogger().Warn("reload after credential change failed; change persisted, manual reload may be needed",
			zap.String("op", op), zap.Error(err))
	}
}

// CreateChannelCredential encrypts and stores a new credential for a channel,
// returning the masked view. The channel must exist.
func (s *LLMProxyService) CreateChannelCredential(channelID uint, purpose, source, envVarName, providerType, label, plaintext string, priority int) (*ChannelCredentialView, error) {
	if s.credentialRepo == nil {
		return nil, fmt.Errorf("credential repository not initialized")
	}
	if channelID == 0 {
		return nil, fmt.Errorf("channel_id is required")
	}
	if purpose == "" {
		purpose = "api"
	}
	if source == "" {
		source = "direct"
	}
	if source == "env" && envVarName == "" {
		return nil, fmt.Errorf("env_var_name is required when source=env")
	}
	if source == "direct" && plaintext == "" {
		return nil, fmt.Errorf("credential is required when source=direct")
	}
	if _, err := s.channelRepo.Get(channelID); err != nil {
		return nil, fmt.Errorf("channel %d not found: %w", channelID, err)
	}
	now := time.Now()
	c := &model.LLMChannelCredential{
		ChannelID:       channelID,
		Purpose:         purpose,
		Source:          source,
		EnvVarName:      envVarName,
		Label:           label,
		Priority:        priority,
		ProviderType:    providerType,
		Status:          "active",
		LastRefreshedAt: &now,
	}
	if source == "direct" {
		if err := s.credentialRepo.Create(c, plaintext); err != nil {
			return nil, fmt.Errorf("create credential: %w", err)
		}
	} else {
		if err := s.credentialRepo.Create(c, ""); err != nil {
			return nil, fmt.Errorf("create credential: %w", err)
		}
	}
	v := toCredentialView(c)
	s.reloadAfterCredentialChange("create")
	return &v, nil
}

// ListChannelCredentials returns the masked credentials for a channel.
func (s *LLMProxyService) ListChannelCredentials(channelID uint) ([]ChannelCredentialView, error) {
	if s.credentialRepo == nil {
		return nil, fmt.Errorf("credential repository not initialized")
	}
	creds, err := s.credentialRepo.ListByChannel(channelID)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	views := make([]ChannelCredentialView, 0, len(creds))
	for i := range creds {
		views = append(views, toCredentialView(&creds[i]))
	}
	return views, nil
}

// UpdateChannelCredential updates a credential's metadata and, when plaintext is
// non-empty, re-encrypts the secret (refreshing LastRefreshedAt).
func (s *LLMProxyService) UpdateChannelCredential(id uint, providerType, status, plaintext, purpose, source, envVarName, label string, priority *int) (*ChannelCredentialView, error) {
	if s.credentialRepo == nil {
		return nil, fmt.Errorf("credential repository not initialized")
	}
	c, err := s.credentialRepo.Get(id)
	if err != nil {
		return nil, fmt.Errorf("credential %d not found: %w", id, err)
	}
	if providerType != "" {
		c.ProviderType = providerType
	}
	if status != "" {
		c.Status = status
	}
	if purpose != "" {
		c.Purpose = purpose
	}
	if source == "direct" && c.Source == "env" {
		c.EnvVarName = ""
	} else if source == "env" && c.Source == "direct" {
		c.CredentialJSON = ""
	}
	if source != "" {
		c.Source = source
	}
	if envVarName != "" {
		c.EnvVarName = envVarName
	}
	if label != "" {
		c.Label = label
	}
	if priority != nil {
		c.Priority = *priority
	}
	if source == "env" && envVarName == "" && c.EnvVarName == "" {
		return nil, fmt.Errorf("env_var_name is required when source=env")
	}
	if plaintext != "" {
		now := time.Now()
		c.LastRefreshedAt = &now
		c.ErrorMessage = ""
		c.Status = "active"
	}
	if err := s.credentialRepo.Update(c, plaintext); err != nil {
		return nil, fmt.Errorf("update credential: %w", err)
	}
	v := toCredentialView(c)
	s.reloadAfterCredentialChange("update")
	return &v, nil
}

// DeleteChannelCredential removes a stored credential.
func (s *LLMProxyService) DeleteChannelCredential(id uint) error {
	if s.credentialRepo == nil {
		return fmt.Errorf("credential repository not initialized")
	}
	if err := s.credentialRepo.Delete(id); err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	s.reloadAfterCredentialChange("delete")
	return nil
}

// GetDecryptedCredential returns the plaintext credential for internal use (e.g.
// re-importing an upstream session). Not exposed directly over the API.
func (s *LLMProxyService) GetDecryptedCredential(id uint) (string, error) {
	if s.credentialRepo == nil {
		return "", fmt.Errorf("credential repository not initialized")
	}
	return s.credentialRepo.GetDecrypted(id)
}

func (s *LLMProxyService) ResolveCredential(channelID uint, purpose string) (string, error) {
	if s.credentialRepo == nil {
		return "", fmt.Errorf("credential repository not initialized")
	}
	creds, err := s.credentialRepo.ListByChannel(channelID)
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
			plain, err := s.credentialRepo.GetDecrypted(c.ID)
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
