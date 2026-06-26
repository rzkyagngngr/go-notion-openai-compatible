package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mughu-id/notionchat/internal/config"
	"github.com/mughu-id/notionchat/internal/credentials"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	settings := &config.Settings{
		APIKey: "sk-test", Host: "127.0.0.1", Port: 8787,
		SessionFile: dir + "/session.json",
		AccountPath: dir + "/account.json",
		ThreadStateDir: dir + "/threads",
		BaseURL: config.DefaultBaseURL,
	}
	return NewServer(settings, credentials.NewStore(settings.SessionFile, settings.AccountPath))
}

func TestHealthz(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestModelsUnauthorized(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHomePage(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "notion_browser_id") {
		t.Fatal("home page missing notion_browser_id field")
	}
}

func TestSessionAPIGet(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["connected"] != false {
		t.Fatalf("expected disconnected: %v", body)
	}
}

func TestConfigRedirectsHome(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status %d", rec.Code)
	}
}