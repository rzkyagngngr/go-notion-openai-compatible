package notionhttp

import (
	"testing"

	http "github.com/bogdanfinn/fhttp"
)

func TestMergeCookieValuesUpdatesToken(t *testing.T) {
	existing := "notion_browser_id=br; notion_user_id=u1; token_v2=old"
	merged, changed := MergeCookieValues(existing, []*http.Cookie{
		{Name: "token_v2", Value: "new"},
	})
	if !changed {
		t.Fatal("expected change")
	}
	if CookieValue(merged, "token_v2") != "new" {
		t.Fatalf("token not updated: %q", merged)
	}
	if CookieValue(merged, "notion_browser_id") != "br" {
		t.Fatalf("browser id lost: %q", merged)
	}
}

func TestMergeCookieValuesNoChange(t *testing.T) {
	existing := "token_v2=same"
	_, changed := MergeCookieValues(existing, []*http.Cookie{
		{Name: "token_v2", Value: "same"},
	})
	if changed {
		t.Fatal("expected no change")
	}
}