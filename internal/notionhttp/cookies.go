package notionhttp

import (
	"net/url"
	"strings"

	http "github.com/bogdanfinn/fhttp"
)

func ParseSetCookieHeaders(headers http.Header) []*http.Cookie {
	var out []*http.Cookie
	for _, line := range headers.Values("Set-Cookie") {
		if c := parseSetCookieLine(line); c != nil {
			out = append(out, c)
		}
	}
	return out
}

func parseSetCookieLine(line string) *http.Cookie {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	parts := strings.Split(line, ";")
	if len(parts) == 0 {
		return nil
	}
	name, value, ok := strings.Cut(strings.TrimSpace(parts[0]), "=")
	if !ok || name == "" {
		return nil
	}
	return &http.Cookie{Name: name, Value: value}
}

func MergeCookieValues(existing string, cookies []*http.Cookie) (string, bool) {
	if len(cookies) == 0 {
		return existing, false
	}
	parsed := parseCookieMap(existing)
	changed := false
	for _, c := range cookies {
		if c == nil || c.Name == "" {
			continue
		}
		if parsed[c.Name] != c.Value {
			parsed[c.Name] = c.Value
			changed = true
		}
	}
	if !changed {
		return existing, false
	}
	return joinCookieMap(parsed), true
}

func parseCookieMap(cookie string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, "=") {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		out[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return out
}

func joinCookieMap(m map[string]string) string {
	order := []string{
		"notion_browser_id", "device_id", "notion_user_id", "notion_users",
		"notion_check_cookie_consent", "notion_locale", "token_v2",
	}
	seen := make(map[string]bool)
	var parts []string
	for _, key := range order {
		if v, ok := m[key]; ok && v != "" {
			parts = append(parts, key+"="+v)
			seen[key] = true
		}
	}
	for key, val := range m {
		if seen[key] || val == "" {
			continue
		}
		parts = append(parts, key+"="+val)
	}
	return strings.Join(parts, "; ")
}

func CookieValue(cookie, name string) string {
	return parseCookieMap(cookie)[name]
}

func SeedJar(jar http.CookieJar, cookie string) error {
	if jar == nil || strings.TrimSpace(cookie) == "" {
		return nil
	}
	u, err := url.Parse("https://app.notion.com/")
	if err != nil {
		return err
	}
	var cookies []*http.Cookie
	for name, value := range parseCookieMap(cookie) {
		cookies = append(cookies, &http.Cookie{Name: name, Value: value, Path: "/"})
	}
	jar.SetCookies(u, cookies)
	return nil
}