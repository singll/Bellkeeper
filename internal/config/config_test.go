package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bellkeeper.yaml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadExpandsEnvInAllStringFields(t *testing.T) {
	t.Setenv("TEST_DB_HOST", "db.internal")
	t.Setenv("TEST_NATS_URL", "nats://nats:4222")
	t.Setenv("TEST_REDIS_PASSWORD", "redis-secret")

	path := writeConfig(t, `
server:
  mode: debug
database:
  host: ${TEST_DB_HOST}
redis:
  password: ${TEST_REDIS_PASSWORD}
nats:
  url: ${TEST_NATS_URL}
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Database.Host != "db.internal" {
		t.Fatalf("database.host = %q, want expanded env", cfg.Database.Host)
	}
	if cfg.NATS.URL != "nats://nats:4222" {
		t.Fatalf("nats.url = %q, want expanded env", cfg.NATS.URL)
	}
	if cfg.Redis.Password != "redis-secret" {
		t.Fatalf("redis.password = %q, want expanded env", cfg.Redis.Password)
	}
}

func TestLoadRejectsUnsafeReleaseConfig(t *testing.T) {
	t.Setenv("BELLKEEPER_CREDENTIAL_KEY", "")
	path := writeConfig(t, `
server:
  mode: release
  api_key: ""
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "server.api_key") {
		t.Fatalf("Load err = %v, want missing api_key error", err)
	}
}
