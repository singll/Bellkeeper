package auth_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/auth"
	"github.com/singll/bellkeeper/internal/model"
)

// stubTokenStore implements auth.LLMTokenStore for middleware tests (no DB).
type stubTokenStore struct {
	byHash       map[string]*model.LLMToken
	modelGroups  map[string]bool
	groupErr     error
	lastUsedID   uint
	lastUsedSeen bool
}

func (s *stubTokenStore) GetByKeyHash(hash string) (*model.LLMToken, error) {
	if t, ok := s.byHash[hash]; ok {
		return t, nil
	}
	return nil, errors.New("not found")
}
func (s *stubTokenStore) IsModelGroupName(name string) (bool, error) {
	if s.groupErr != nil {
		return false, s.groupErr
	}
	return s.modelGroups[name], nil
}
func (s *stubTokenStore) CountRequestsToday(uint) (int, error) { return 0, nil }
func (s *stubTokenStore) TokensUsedToday(uint) (int, error)    { return 0, nil }
func (s *stubTokenStore) CostThisMonthCents(uint) (int, error) { return 0, nil }
func (s *stubTokenStore) UpdateLastUsed(id uint) error {
	s.lastUsedID = id
	s.lastUsedSeen = true
	return nil
}

func runAuth(store auth.LLMTokenStore, serverKey, bearer string) (*gin.Context, *httptest.ResponseRecorder) {
	return runAuthWithBody(store, serverKey, bearer, "")
}

func runAuthWithBody(store auth.LLMTokenStore, serverKey, bearer, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", reader)
	c.Request.Header.Set("Authorization", "Bearer "+bearer)
	c.Request.Header.Set("Content-Type", "application/json")
	auth.LLMTokenAuth(store, serverKey)(c)
	return c, w
}

// When the server key matches the seeded "default" token, the request resolves to
// that token (non-zero id, caller "default") so its usage is billed (Tier 8).
func TestLLMTokenAuth_ServerKeyResolvesToDefaultToken(t *testing.T) {
	const serverKey = "sk-bk-server-secret"
	def := &model.LLMToken{ID: 7, CallerID: "default", Enabled: true, KeyHash: model.HashKey(serverKey)}
	store := &stubTokenStore{byHash: map[string]*model.LLMToken{model.HashKey(serverKey): def}}

	c, _ := runAuth(store, serverKey, serverKey)
	if c.IsAborted() {
		t.Fatal("server key with a default token should not be rejected")
	}
	id := auth.GetCallerIdentity(c)
	if id.TokenID != 7 {
		t.Errorf("token_id = %d, want 7 (billed via default token)", id.TokenID)
	}
	if id.CallerID != "default" {
		t.Errorf("caller_id = %q, want \"default\"", id.CallerID)
	}
	if !store.lastUsedSeen || store.lastUsedID != 7 {
		t.Errorf("UpdateLastUsed not called for default token (seen=%v id=%d)", store.lastUsedSeen, store.lastUsedID)
	}
}

// Without a seeded default token, the server key falls back to the legacy admin
// bypass: authenticated but unbilled (token_id 0).
func TestLLMTokenAuth_ServerKeyFallbackBypass(t *testing.T) {
	const serverKey = "sk-bk-server-secret"
	store := &stubTokenStore{byHash: map[string]*model.LLMToken{}}

	c, _ := runAuth(store, serverKey, serverKey)
	if c.IsAborted() {
		t.Fatal("server key should be accepted even without a default token")
	}
	id := auth.GetCallerIdentity(c)
	if id.TokenID != 0 {
		t.Errorf("token_id = %d, want 0 (legacy bypass)", id.TokenID)
	}
	if id.CallerID != "server" {
		t.Errorf("caller_id = %q, want \"server\"", id.CallerID)
	}
}

// An unknown key (neither the server key nor a known token) is rejected with 401.
func TestLLMTokenAuth_UnknownKeyRejected(t *testing.T) {
	store := &stubTokenStore{byHash: map[string]*model.LLMToken{}}
	c, w := runAuth(store, "sk-bk-server-secret", "sk-bk-bogus")
	if !c.IsAborted() {
		t.Fatal("unknown key should be rejected")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestLLMTokenAuth_ModelAllowlist(t *testing.T) {
	const key = "sk-bk-token"
	token := &model.LLMToken{ID: 10, CallerID: "client", Enabled: true}
	token.SetAllowedModels([]string{"gpt-4o"})
	store := &stubTokenStore{byHash: map[string]*model.LLMToken{model.HashKey(key): token}}

	c, _ := runAuthWithBody(store, "server-key", key, `{"model":"gpt-4o"}`)
	if c.IsAborted() {
		t.Fatal("allowed model should pass")
	}

	c, w := runAuthWithBody(store, "server-key", key, `{"model":"gpt-3.5"}`)
	if !c.IsAborted() {
		t.Fatal("disallowed model should be rejected")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestLLMTokenAuth_GroupAllowlist(t *testing.T) {
	const key = "sk-bk-token"
	token := &model.LLMToken{ID: 11, CallerID: "client", Enabled: true}
	token.SetAllowedGroups([]string{"pool-coding"})
	store := &stubTokenStore{
		byHash:      map[string]*model.LLMToken{model.HashKey(key): token},
		modelGroups: map[string]bool{"pool-coding": true, "pool-pkb": true},
	}

	c, _ := runAuthWithBody(store, "server-key", key, `{"model":"pool-coding"}`)
	if c.IsAborted() {
		t.Fatal("allowed model group should pass")
	}

	c, w := runAuthWithBody(store, "server-key", key, `{"model":"pool-pkb"}`)
	if !c.IsAborted() {
		t.Fatal("disallowed model group should be rejected")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestLLMTokenAuth_EmptyAllowlistsAllowKnownGroup(t *testing.T) {
	const key = "sk-bk-token"
	token := &model.LLMToken{ID: 12, CallerID: "client", Enabled: true}
	store := &stubTokenStore{
		byHash:      map[string]*model.LLMToken{model.HashKey(key): token},
		modelGroups: map[string]bool{"pool-open": true},
	}

	c, _ := runAuthWithBody(store, "server-key", key, `{"model":"pool-open"}`)
	if c.IsAborted() {
		t.Fatal("empty allowlists should allow all model groups")
	}
}

func TestLLMTokenAuth_EmptyServerKeyDoesNotBypass(t *testing.T) {
	store := &stubTokenStore{byHash: map[string]*model.LLMToken{}}
	c, w := runAuth(store, "", "")
	if !c.IsAborted() {
		t.Fatal("empty bearer must not bypass when server key is empty")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}
