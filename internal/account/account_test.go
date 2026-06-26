package account

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBrowserCookie(t *testing.T) {
	cookie := "notion_browser_id=abc-123; token_v2=v03%3Atest; notion_user_id=user1"
	parsed := ParseBrowserCookie(cookie)
	if parsed["notion_browser_id"] != "abc-123" {
		t.Fatalf("browser_id: got %q", parsed["notion_browser_id"])
	}
	if parsed["token_v2"] != "v03%3Atest" {
		t.Fatalf("token_v2: got %q", parsed["token_v2"])
	}
}

func TestBuildCookieHeader(t *testing.T) {
	acc := &NotionAccount{
		TokenV2: "tok", BrowserID: "br", DeviceID: "dev", UserID: "uid",
	}
	h := BuildCookieHeader(acc)
	if !containsAll(h, "notion_browser_id=br", "token_v2=tok", "device_id=dev") {
		t.Fatalf("cookie header: %q", h)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "acc.json")
	orig := &NotionAccount{
		TokenV2: "tok", UserID: "uid", SpaceID: "space",
		BrowserID: "br", DeviceID: "dev",
	}
	if err := Save(orig, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TokenV2 != "tok" || loaded.SpaceID != "space" {
		t.Fatalf("loaded mismatch: %+v", loaded)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}