package llmgateway

import (
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// ConfigReloader is the runtime-side hook invoked after a persisted config
// change. The config service never reads/owns routing snapshots; it only writes
// rows and asks the runtime to reload them.
type ConfigReloader interface {
	Reload() error
}

// LLMConfigService owns channel/group/credential configuration writes. Keeping
// these out of LLMProxyService means the runtime only serves reads, snapshots
// and Reload, while the handler depends on this service for mutations.
type LLMConfigService struct {
	channelRepo    *repository.LLMChannelRepository
	groupRepo      *repository.LLMModelGroupRepository
	credentialRepo *repository.LLMChannelCredentialRepository
	reloader       ConfigReloader
}

func NewLLMConfigService(
	channelRepo *repository.LLMChannelRepository,
	groupRepo *repository.LLMModelGroupRepository,
	credentialRepo *repository.LLMChannelCredentialRepository,
	reloader ConfigReloader,
) *LLMConfigService {
	return &LLMConfigService{
		channelRepo:    channelRepo,
		groupRepo:      groupRepo,
		credentialRepo: credentialRepo,
		reloader:       reloader,
	}
}

func (s *LLMConfigService) reload() error {
	if s.reloader == nil {
		return nil
	}
	return s.reloader.Reload()
}

// --- Channel config ---

func (s *LLMConfigService) ListChannelConfigs() ([]ChannelConfigView, error) {
	channels, err := s.channelRepo.List()
	if err != nil {
		return nil, err
	}
	views := make([]ChannelConfigView, 0, len(channels))
	for i := range channels {
		views = append(views, channelToView(s.credentialRepo, &channels[i]))
	}
	return views, nil
}

func (s *LLMConfigService) CreateChannel(ch *model.LLMChannel) error {
	if err := validateChannel(ch); err != nil {
		return err
	}
	if _, err := s.channelRepo.GetByName(ch.Name); err == nil {
		return invalidf("channel %q already exists", ch.Name)
	}
	if err := s.channelRepo.Create(ch); err != nil {
		return err
	}
	return s.reload()
}

func (s *LLMConfigService) UpdateChannel(ch *model.LLMChannel) error {
	if _, err := s.channelRepo.Get(ch.ID); err != nil {
		return fmt.Errorf("channel %d not found: %w", ch.ID, err)
	}
	if err := validateChannel(ch); err != nil {
		return err
	}
	if err := s.channelRepo.Update(ch); err != nil {
		return err
	}
	return s.reload()
}

func (s *LLMConfigService) DeleteChannel(id uint) error {
	ch, err := s.channelRepo.Get(id)
	if err != nil {
		return fmt.Errorf("channel %d not found: %w", id, err)
	}
	groups, err := s.groupRepo.List()
	if err != nil {
		return fmt.Errorf("check channel references: %w", err)
	}
	for _, g := range groups {
		for _, m := range g.Members {
			if m.ChannelName == ch.Name {
				return invalidf("channel %q is used by model group %q; remove that member before deleting the channel", ch.Name, g.Name)
			}
		}
	}
	if err := s.channelRepo.Delete(id); err != nil {
		return err
	}
	return s.reload()
}

// --- Model group config ---

func (s *LLMConfigService) ListGroupConfigs() ([]model.LLMModelGroup, error) {
	return s.groupRepo.List()
}

func (s *LLMConfigService) CreateGroup(g *model.LLMModelGroup) error {
	if err := validateGroup(g, s.channelRepo); err != nil {
		return err
	}
	if err := s.groupRepo.Create(g); err != nil {
		return err
	}
	return s.reload()
}

func (s *LLMConfigService) UpdateGroup(g *model.LLMModelGroup) error {
	if _, err := s.groupRepo.Get(g.ID); err != nil {
		return fmt.Errorf("group %d not found: %w", g.ID, err)
	}
	if err := validateGroup(g, s.channelRepo); err != nil {
		return err
	}
	if err := s.groupRepo.Update(g); err != nil {
		return err
	}
	return s.reload()
}

func (s *LLMConfigService) DeleteGroup(id uint) error {
	if err := s.groupRepo.Delete(id); err != nil {
		return err
	}
	return s.reload()
}

// --- Channel credentials ---

// reloadAfterCredentialChange reloads runtime config so a credential mutation
// takes effect immediately — a channel's API key is resolved into
// Channel.Config only at load time, so without this a newly added/edited key
// would not be used until the next restart or explicit reload. The credential
// row is already persisted, so a reload failure is logged rather than
// propagated (Channel/Group CRUD reload alike).
func (s *LLMConfigService) reloadAfterCredentialChange(op string) {
	if err := s.reload(); err != nil {
		middleware.GetLogger().Warn("reload after credential change failed; change persisted, manual reload may be needed",
			zap.String("op", op), zap.Error(err))
	}
}

func (s *LLMConfigService) CreateChannelCredential(channelID uint, purpose, source, envVarName, providerType, label, plaintext string, priority int) (*ChannelCredentialView, error) {
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
	secret := ""
	if source == "direct" {
		secret = plaintext
	}
	if err := s.credentialRepo.Create(c, secret); err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}
	v := toCredentialView(c)
	s.reloadAfterCredentialChange("create")
	return &v, nil
}

// ListChannelCredentials returns the masked credentials for a channel.
func (s *LLMConfigService) ListChannelCredentials(channelID uint) ([]ChannelCredentialView, error) {
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
func (s *LLMConfigService) UpdateChannelCredential(id uint, providerType, status, plaintext, purpose, source, envVarName, label string, priority *int) (*ChannelCredentialView, error) {
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
func (s *LLMConfigService) DeleteChannelCredential(id uint) error {
	if s.credentialRepo == nil {
		return fmt.Errorf("credential repository not initialized")
	}
	if err := s.credentialRepo.Delete(id); err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	s.reloadAfterCredentialChange("delete")
	return nil
}
