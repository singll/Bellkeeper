package crypto

import (
	"os"
	"strings"
	"testing"
)

// TestEncryptDecryptRoundTrip exercises the real AES-256-GCM path: ciphertext must
// differ from plaintext, carry the "gcm:" prefix, round-trip back to the original,
// use a fresh nonce per call, and treat unprefixed values as plaintext on decrypt.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	// Key must be set before any (de)cryption so initCipher latches enabled.
	os.Setenv(credentialKeyEnv, "test-credential-key-please-rotate")
	defer os.Unsetenv(credentialKeyEnv)

	if !Enabled() {
		t.Fatal("encryption should be enabled when the key is set")
	}

	plain := "sk-bk-supersecret-value-1234567890"
	ct, err := Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ct == plain {
		t.Fatal("ciphertext equals plaintext — value was not encrypted")
	}
	if !strings.HasPrefix(ct, cipherPrefix) {
		t.Fatalf("ciphertext missing %q prefix: %q", cipherPrefix, ct)
	}

	got, err := Decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
	}

	// Fresh nonce per call → identical plaintext yields distinct ciphertext.
	if ct2, _ := Encrypt(plain); ct2 == ct {
		t.Fatal("nonce reuse: two encryptions produced identical ciphertext")
	}

	// Unprefixed values (stored while encryption was disabled) pass through.
	if pass, err := Decrypt("legacy-plaintext"); err != nil || pass != "legacy-plaintext" {
		t.Fatalf("passthrough decrypt failed: got %q err %v", pass, err)
	}
}
