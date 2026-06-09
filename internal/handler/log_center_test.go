package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLogCenterAdminRejectsEmptyServerKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewLogCenterHandler(nil, "")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/log-center/sources", strings.NewReader(`{"name":"x","source_type":"app"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RegisterSource(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestLogCenterAdminRejectsMissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewLogCenterHandler(nil, "server-secret")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/log-center/sources", strings.NewReader(`{"name":"x","source_type":"app"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RegisterSource(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
