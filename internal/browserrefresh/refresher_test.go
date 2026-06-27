package browserrefresh

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

func TestCookiesToHeader(t *testing.T) {
	cookies := []*proto.NetworkCookie{
		{Name: "token_v2", Value: "tok-abc"},
		{Name: "notion_browser_id", Value: "br-1"},
		{Name: "notion_user_id", Value: "user-1"},
	}
	header := cookiesToHeader(cookies)
	if header == "" {
		t.Fatal("expected header")
	}
	if !contains(header, "token_v2=tok-abc") {
		t.Fatalf("header: %s", header)
	}
}

func TestLoadConfigDisabledWithoutChromium(t *testing.T) {
	t.Setenv("NOTION_BROWSER_MODE", "headless")
	t.Setenv("NOTION_BROWSER_CHROMIUM_PATH", "/nonexistent/chromium")
	cfg := LoadConfig()
	if cfg.Mode != ModeDisabled {
		t.Fatalf("mode=%s", cfg.Mode)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}