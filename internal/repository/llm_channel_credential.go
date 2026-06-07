package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/crypto"
	"gorm.io/gorm"
)

// LLMChannelCredentialRepository manages encrypted per-channel provider credentials.
// Plaintext crosses this boundary only via the explicit (de)cryption helpers; the
// stored CredentialJSON column is always ciphertext (when encryption is enabled).
type LLMChannelCredentialRepository struct {
	db *gorm.DB
}

func NewLLMChannelCredentialRepository(db *gorm.DB) *LLMChannelCredentialRepository {
	return &LLMChannelCredentialRepository{db: db}
}

// Create encrypts the plaintext payload and inserts a new credential row.
func (r *LLMChannelCredentialRepository) Create(c *model.LLMChannelCredential, plaintext string) error {
	enc, err := crypto.Encrypt(plaintext)
	if err != nil {
		return err
	}
	c.CredentialJSON = enc
	return r.db.Create(c).Error
}

// Update saves metadata; when plaintext is non-empty the credential is re-encrypted,
// otherwise the existing ciphertext is preserved.
func (r *LLMChannelCredentialRepository) Update(c *model.LLMChannelCredential, plaintext string) error {
	if plaintext != "" {
		enc, err := crypto.Encrypt(plaintext)
		if err != nil {
			return err
		}
		c.CredentialJSON = enc
	}
	return r.db.Save(c).Error
}

// Get returns a credential row (CredentialJSON still encrypted).
func (r *LLMChannelCredentialRepository) Get(id uint) (*model.LLMChannelCredential, error) {
	var c model.LLMChannelCredential
	if err := r.db.First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListByChannel returns all credential rows for a channel (CredentialJSON encrypted).
func (r *LLMChannelCredentialRepository) ListByChannel(channelID uint) ([]model.LLMChannelCredential, error) {
	var creds []model.LLMChannelCredential
	err := r.db.Where("channel_id = ?", channelID).Order("priority ASC, id ASC").Find(&creds).Error
	return creds, err
}

// GetDecrypted returns the plaintext credential for internal (non-API) use.
func (r *LLMChannelCredentialRepository) GetDecrypted(id uint) (string, error) {
	c, err := r.Get(id)
	if err != nil {
		return "", err
	}
	return crypto.Decrypt(c.CredentialJSON)
}

// Delete removes a credential row.
func (r *LLMChannelCredentialRepository) Delete(id uint) error {
	return r.db.Delete(&model.LLMChannelCredential{}, id).Error
}
