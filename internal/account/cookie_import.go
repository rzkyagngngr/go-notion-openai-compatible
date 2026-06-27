package account

import (
	"encoding/json"
	"strings"
)

type exportedCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

var sessionCookieNames = map[string]bool{
	"token_v2":                    true,
	"notion_browser_id":           true,
	"device_id":                   true,
	"notion_user_id":              true,
	"notion_users":                true,
	"notion_check_cookie_consent": true,
	"notion_locale":               true,
	"csrf":                        true,
}

// ParseCookieImport converts Cookie Editor JSON export into a semicolon cookie header.
// Returns the original string and false when input is not JSON cookies.
func ParseCookieImport(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || (!strings.HasPrefix(raw, "[") && !strings.HasPrefix(raw, "{")) {
		return raw, false
	}

	cookies, ok := decodeExportedCookies(raw)
	if !ok || len(cookies) == 0 {
		return raw, false
	}

	type entry struct {
		value string
		score int
	}
	best := make(map[string]entry)
	for _, c := range cookies {
		name := strings.TrimSpace(c.Name)
		value := strings.TrimSpace(c.Value)
		if name == "" || value == "" || !sessionCookieNames[name] {
			continue
		}
		score := 0
		d := strings.ToLower(c.Domain)
		if strings.Contains(d, "app.notion.com") {
			score = 2
		} else if strings.Contains(d, "notion.com") {
			score = 1
		}
		if prev, exists := best[name]; !exists || score > prev.score {
			best[name] = entry{value: value, score: score}
		}
	}

	token := best["token_v2"].value
	if token == "" {
		return raw, false
	}

	browserID := best["notion_browser_id"].value
	deviceID := best["device_id"].value
	userID := best["notion_user_id"].value
	if userID != "" {
		return BuildCookieFromParts(browserID, deviceID, userID, token), true
	}

	order := []string{
		"notion_browser_id", "device_id", "notion_user_id", "notion_users",
		"notion_check_cookie_consent", "notion_locale", "csrf", "token_v2",
	}
	var parts []string
	for _, name := range order {
		if e, ok := best[name]; ok {
			parts = append(parts, name+"="+e.value)
		}
	}
	return strings.Join(parts, "; "), true
}

func decodeExportedCookies(raw string) ([]exportedCookie, bool) {
	if strings.HasPrefix(raw, "[") {
		var cookies []exportedCookie
		if err := json.Unmarshal([]byte(raw), &cookies); err != nil {
			return nil, false
		}
		return cookies, true
	}
	var wrap struct {
		Cookies []exportedCookie `json:"cookies"`
	}
	if err := json.Unmarshal([]byte(raw), &wrap); err != nil || len(wrap.Cookies) == 0 {
		return nil, false
	}
	return wrap.Cookies, true
}