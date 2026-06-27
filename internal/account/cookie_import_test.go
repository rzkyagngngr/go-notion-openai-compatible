package account

import (
	"strings"
	"testing"
)

func TestParseCookieImportJSON(t *testing.T) {
	raw := `[
	  {"domain":".notion.com","name":"notion_browser_id","value":"f7625b56-a0f2-428f-aed7-9df07c69c5a2"},
	  {"domain":".app.notion.com","name":"device_id","value":"38bd872b-594c-8102-81f1-003bf468a6f1"},
	  {"domain":".app.notion.com","name":"notion_user_id","value":"388d872b-594c-81dd-9130-0002fe131c48"},
	  {"domain":".app.notion.com","name":"token_v2","value":"v03%3Atest-token-value"}
	]`
	cookie, ok := ParseCookieImport(raw)
	if !ok {
		t.Fatal("expected JSON cookies to parse")
	}
	if !strings.Contains(cookie, "token_v2=v03%3Atest-token-value") {
		t.Fatalf("missing token: %s", cookie)
	}
	if !strings.Contains(cookie, "notion_browser_id=f7625b56-a0f2-428f-aed7-9df07c69c5a2") {
		t.Fatalf("missing browser id: %s", cookie)
	}
	if !strings.Contains(cookie, "notion_user_id=388d872b-594c-81dd-9130-0002fe131c48") {
		t.Fatalf("missing user id: %s", cookie)
	}
}

func TestParseCookieImportPlainPassthrough(t *testing.T) {
	raw := "token_v2=abc; notion_browser_id=br"
	cookie, ok := ParseCookieImport(raw)
	if ok {
		t.Fatal("plain cookie should not be converted")
	}
	if cookie != raw {
		t.Fatalf("got %q", cookie)
	}
}