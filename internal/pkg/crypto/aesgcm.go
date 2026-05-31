// Package crypto provides symmetric encryption for sensitive credentials at rest.
//
// It uses AES-256-GCM with a key derived (SHA-256) from the BELLKEEPER_CREDENTIAL_KEY
// environment variable. If that variable is unset, encryption is DISABLED: Encrypt
// returns its input unchanged (a startup warning is logged on first use) so the
// system degrades to a clearly-flagged passthrough rather than failing to boot.
//
// Ciphertext format: "gcm:" + base64(nonce || ciphertext||tag). Values lacking the
// "gcm:" prefix are treated as plaintext on Decrypt, so toggling the key on or off
// does not corrupt previously stored rows.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

const (
	credentialKeyEnv = "BELLKEEPER_CREDENTIAL_KEY"
	cipherPrefix     = "gcm:"
)

var (
	once    sync.Once
	gcm     cipher.AEAD
	enabled bool
)

func initCipher() {
	once.Do(func() {
		key := os.Getenv(credentialKeyEnv)
		if key == "" {
			middleware.GetLogger().Warn("credential encryption disabled: " +
				credentialKeyEnv + " is unset; credentials are stored as plaintext")
			return
		}
		sum := sha256.Sum256([]byte(key))
		block, err := aes.NewCipher(sum[:])
		if err != nil {
			middleware.GetLogger().Error("failed to init credential cipher", zap.Error(err))
			return
		}
		g, err := cipher.NewGCM(block)
		if err != nil {
			middleware.GetLogger().Error("failed to init GCM", zap.Error(err))
			return
		}
		gcm = g
		enabled = true
	})
}

// Enabled reports whether credential encryption is active (key configured).
func Enabled() bool {
	initCipher()
	return enabled
}

// Encrypt returns the AES-256-GCM ciphertext of plaintext, prefixed "gcm:" and
// base64-encoded. If encryption is disabled, plaintext is returned unchanged.
func Encrypt(plaintext string) (string, error) {
	initCipher()
	if !enabled {
		return plaintext, nil
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return cipherPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. Values without the "gcm:" prefix are returned as-is
// (they were stored as plaintext while encryption was disabled).
func Decrypt(ciphertext string) (string, error) {
	if len(ciphertext) < len(cipherPrefix) || ciphertext[:len(cipherPrefix)] != cipherPrefix {
		return ciphertext, nil
	}
	initCipher()
	if !enabled {
		return "", errors.New("credential is encrypted but " + credentialKeyEnv + " is not set")
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext[len(cipherPrefix):])
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
