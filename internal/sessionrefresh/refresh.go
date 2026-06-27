package sessionrefresh

import (
	"log"
	"strings"

	"github.com/mughu-id/notionchat/internal/account"
	"github.com/mughu-id/notionchat/internal/client"
	"github.com/mughu-id/notionchat/internal/config"
	"github.com/mughu-id/notionchat/internal/ndjson"
	"github.com/mughu-id/notionchat/internal/notionhttp"
)

const probePrompt = "ping"

func IsStaleInferenceLine(line string) bool {
	line = strings.TrimSpace(line)
	return line == "[" || line == "[]"
}

func IsStaleParseResult(result *ndjson.ParseResult) bool {
	if result == nil {
		return true
	}
	if result.Text != "" || len(result.ToolCalls) > 0 {
		return false
	}
	for _, sample := range result.SampleLines {
		if IsStaleInferenceLine(sample) {
			return true
		}
	}
	if result.LineCount == 1 && result.JSONFailures == 1 {
		return true
	}
	return false
}

func RotateViaLoadUserContent(acc *account.NotionAccount) (string, bool, error) {
	httpClient, err := notionhttp.NewClient()
	if err != nil {
		return "", false, err
	}
	defer httpClient.Close()

	cookie := account.BuildCookieHeader(acc)
	headers := map[string]string{
		"accept":                    "application/json",
		"content-type":              "application/json",
		"notion-audit-log-platform": "web",
		"notion-client-version":     acc.ClientVersion,
		"origin":                    "https://app.notion.com",
		"referer":                   "https://app.notion.com/",
		"user-agent":                acc.UserAgent,
		"x-notion-active-user-header": acc.UserID,
		"cookie":                    cookie,
	}
	body := map[string]any{
		"cursor": map[string]any{"stack": []any{}},
		"limit":  5,
	}
	_, status, _, setCookies, err := httpClient.PostJSONWithCookies(
		config.DefaultBaseURL+"/loadUserContent", body, headers,
	)
	if err != nil {
		return "", false, err
	}
	if status != 200 {
		return "", false, nil
	}
	merged, changed := notionhttp.MergeCookieValues(cookie, setCookies)
	if !changed {
		return "", false, nil
	}
	oldToken := notionhttp.CookieValue(cookie, "token_v2")
	newToken := notionhttp.CookieValue(merged, "token_v2")
	if newToken != "" && newToken != oldToken {
		log.Printf("Notion token_v2 rotated via loadUserContent (%s -> %s)",
			maskToken(oldToken), maskToken(newToken))
		return merged, true, nil
	}
	if changed {
		return merged, true, nil
	}
	return "", false, nil
}

func ProbeInference(acc *account.NotionAccount) bool {
	settings := config.Get()
	c := &client.NotionAIClient{
		Account:        acc,
		BaseURL:        settings.BaseURL,
		ThreadStateDir: settings.ThreadStateDir,
	}
	result, err := c.Complete(probePrompt, "", acc.DefaultModel, "", probePrompt, false, false, nil)
	if err != nil {
		return false
	}
	return strings.TrimSpace(result.Text) != ""
}

func maskToken(token string) string {
	if token == "" {
		return "••••"
	}
	if len(token) <= 12 {
		return "••••"
	}
	return token[:6] + "••••" + token[len(token)-4:]
}