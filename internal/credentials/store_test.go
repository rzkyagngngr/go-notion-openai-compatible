package credentials

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCookieFromFields(t *testing.T) {
	input := SessionInput{NotionBrowserID: "br-1", TokenV2: "tok-1"}
	cookie := buildCookie(normalizeInput(input))
	if !strings.Contains(cookie, "notion_browser_id=br-1") {
		t.Fatalf("cookie: %s", cookie)
	}
	if !strings.Contains(cookie, "token_v2=tok-1") {
		t.Fatalf("cookie: %s", cookie)
	}
}

func TestMaskToken(t *testing.T) {
	if maskToken("short") == "" {
		t.Fatal("expected mask")
	}
	m := maskToken("v03%3Averylongtokenvalue")
	if !strings.Contains(m, "••••") {
		t.Fatalf("mask: %s", m)
	}
}

func TestPersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "session.json")
	store := NewStore(sessionFile, filepath.Join(dir, "account.json"), nil)

	store.mu.Lock()
	store.raw = SessionInput{NotionBrowserID: "br", TokenV2: "tok"}
	store.connectedAt = "2026-01-01T00:00:00Z"
	store.updatedAt = "2026-01-01T00:00:00Z"
	store.mu.Unlock()

	if err := store.persist(); err != nil {
		t.Fatal(err)
	}

	loaded := NewStore(sessionFile, filepath.Join(dir, "account.json"), nil)
	st := loaded.Status()
	if st.NotionBrowserID != "br" {
		t.Fatalf("status: %+v", st)
	}
}