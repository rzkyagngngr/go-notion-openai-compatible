package credentials

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeInputJSONCookies(t *testing.T) {
	raw := `[{"name":"notion_browser_id","value":"br-1","domain":".notion.com"},{"name":"notion_user_id","value":"uid-1","domain":".app.notion.com"},{"name":"token_v2","value":"tok-1","domain":".app.notion.com"}]`
	out := normalizeInput(SessionInput{Cookie: raw})
	if out.TokenV2 != "tok-1" {
		t.Fatalf("token: %q", out.TokenV2)
	}
	if out.NotionBrowserID != "br-1" {
		t.Fatalf("browser: %q", out.NotionBrowserID)
	}
}

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