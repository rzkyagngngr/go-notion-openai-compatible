package browserrefresh

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod/lib/proto"
	"github.com/mughu-id/notionchat/internal/account"
)

var cookieDomains = []string{
	"https://www.notion.so",
	"https://notion.so",
	"https://app.notion.com",
}

func cookiesToHeader(cookies []*proto.NetworkCookie) string {
	merged := make(map[string]string)
	for _, c := range cookies {
		if c == nil || c.Name == "" {
			continue
		}
		merged[c.Name] = c.Value
	}
	token := merged["token_v2"]
	if token == "" {
		return ""
	}
	return account.BuildCookieFromParts(
		merged["notion_browser_id"],
		merged["device_id"],
		merged["notion_user_id"],
		token,
	)
}

func hasToken(cookies []*proto.NetworkCookie) bool {
	for _, c := range cookies {
		if c != nil && c.Name == "token_v2" && strings.TrimSpace(c.Value) != "" {
			return true
		}
	}
	return false
}

func mergeCookieSets(sets ...[]*proto.NetworkCookie) []*proto.NetworkCookie {
	out := make([]*proto.NetworkCookie, 0)
	seen := make(map[string]bool)
	for _, set := range sets {
		for _, c := range set {
			if c == nil {
				continue
			}
			key := fmt.Sprintf("%s|%s", c.Domain, c.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, c)
		}
	}
	return out
}