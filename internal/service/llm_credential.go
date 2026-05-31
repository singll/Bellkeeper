package service

import (
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/crypto"
)

// ChannelCredentialView is the API-safe projection of a stored credential. The
// encrypted CredentialJSON is never exposed; CredentialPreview reveals only the
// first/last few characters so operators can recognize which key is stored.
type ChannelCredentialView struct {
	model.LLMChannelCredential
	CredentialPreview string `json:"credential_preview"`
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
	preview := ""
	if plain, err := crypto.Decrypt(c.CredentialJSON); err == nil {
		preview = maskSecret(plain)
	} else {
		preview = "<undecryptable>"
	}
	v := ChannelCredentialView{LLMChannelCredential: *c, CredentialPreview: preview}
	v.CredentialJSON = "" // defense-in-depth: never carry ciphertext into the DTO
	return v
}

// CreateChannelCredential encrypts and stores a new credential for a channel,
// returning the masked view. The channel must exist.
func (s *LLMProxyService) CreateChannelCredential(channelID uint, providerType, plaintext string) (*ChannelCredentialView, error) {
	if s.credentialRepo == nil {
		return nil, fmt.Errorf("credential repository not initialized")
	}
	if channelID == 0 {
		return nil, fmt.Errorf("channel_id is required")
	}
	if plaintext == "" {
		return nil, fmt.Errorf("credential is required")
	}
	if _, err := s.channelRepo.Get(channelID); err != nil {
		return nil, fmt.Errorf("channel %d not found: %w", channelID, err)
	}
	now := time.Now()
	c := &model.LLMChannelCredential{
		ChannelID:       channelID,
		ProviderType:    providerType,
		Status:          "active",
		LastRefreshedAt: &now,
	}
	if err := s.credentialRepo.Create(c, plaintext); err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}
	v := toCredentialView(c)
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
func (s *LLMProxyService) UpdateChannelCredential(id uint, providerType, status, plaintext string) (*ChannelCredentialView, error) {
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
