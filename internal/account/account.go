package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mughu-id/notionchat/internal/errors"
)

const DefaultClientVersion = "23.13.20260616.2105"

type NotionAccount struct {
	TokenV2       string            `json:"token_v2"`
	FullCookie    string            `json:"full_cookie,omitempty"`
	UserID        string            `json:"user_id,omitempty"`
	UserName      string            `json:"user_name,omitempty"`
	UserEmail     string            `json:"user_email,omitempty"`
	SpaceID       string            `json:"space_id,omitempty"`
	SpaceName     string            `json:"space_name,omitempty"`
	SpaceViewID   string            `json:"space_view_id,omitempty"`
	BrowserID     string            `json:"browser_id,omitempty"`
	DeviceID      string            `json:"device_id,omitempty"`
	ClientVersion string            `json:"client_version,omitempty"`
	UserAgent     string            `json:"user_agent,omitempty"`
	Timezone      string            `json:"timezone,omitempty"`
	DefaultModel  string            `json:"default_model,omitempty"`
	Extras        map[string]any    `json:"-"`
	unknown       map[string]any    `json:"-"`
}

func DefaultUserAgent() string {
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
}

func (a *NotionAccount) applyDefaults() {
	if a.ClientVersion == "" {
		a.ClientVersion = DefaultClientVersion
	}
	if a.UserAgent == "" {
		a.UserAgent = DefaultUserAgent()
	}
	if a.Timezone == "" {
		a.Timezone = "America/Los_Angeles"
	}
	if a.DefaultModel == "" {
		a.DefaultModel = "ambrosia-tart-high"
	}
}

func ParseBrowserCookie(cookie string) map[string]string {
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

func BuildCookieFromParts(browserID, deviceID, userID, tokenV2 string) string {
	parts := []string{
		"notion_browser_id=" + browserID,
		"device_id=" + deviceID,
		"notion_user_id=" + userID,
		fmt.Sprintf(`notion_users=[%%22%s%%22]`, userID),
		"notion_check_cookie_consent=false",
		"notion_locale=en-US/autodetect",
		"token_v2=" + tokenV2,
	}
	return strings.Join(parts, "; ")
}

func BuildCookieHeader(acc *NotionAccount) string {
	if acc.TokenV2 == "" {
		if acc.FullCookie != "" {
			return acc.FullCookie
		}
		return ""
	}
	browserID := acc.BrowserID
	deviceID := acc.DeviceID
	userID := acc.UserID
	token := acc.TokenV2
	if acc.FullCookie != "" {
		parsed := ParseBrowserCookie(acc.FullCookie)
		if browserID == "" {
			browserID = parsed["notion_browser_id"]
		}
		if deviceID == "" {
			deviceID = parsed["device_id"]
		}
		if userID == "" {
			userID = parsed["notion_user_id"]
		}
		if token == "" {
			token = parsed["token_v2"]
		}
	}
	// Rebuild when we know user_id — stale full_cookie often has empty notion_user_id
	// which breaks runInferenceTranscript (Notion returns "[]" with no events).
	if userID != "" && token != "" {
		return BuildCookieFromParts(browserID, deviceID, userID, token)
	}
	if acc.FullCookie != "" {
		return acc.FullCookie
	}
	return BuildCookieFromParts(acc.BrowserID, acc.DeviceID, acc.UserID, acc.TokenV2)
}

func Load(path string) (*NotionAccount, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("Account file not found: "+path, 500)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New("Invalid account JSON: "+err.Error(), 500)
	}
	for _, field := range []string{"token_v2", "user_id", "space_id"} {
		if v, ok := raw[field].(string); !ok || v == "" {
			return nil, errors.New("Account file missing required field: "+field, 500)
		}
	}
	known := map[string]bool{
		"token_v2": true, "full_cookie": true, "user_id": true, "user_name": true,
		"user_email": true, "space_id": true, "space_name": true, "space_view_id": true,
		"browser_id": true, "device_id": true, "client_version": true, "user_agent": true,
		"timezone": true, "default_model": true,
	}
	acc := &NotionAccount{Extras: make(map[string]any)}
	if v, ok := raw["token_v2"].(string); ok {
		acc.TokenV2 = v
	}
	if v, ok := raw["full_cookie"].(string); ok {
		acc.FullCookie = v
	}
	if v, ok := raw["user_id"].(string); ok {
		acc.UserID = v
	}
	if v, ok := raw["user_name"].(string); ok {
		acc.UserName = v
	}
	if v, ok := raw["user_email"].(string); ok {
		acc.UserEmail = v
	}
	if v, ok := raw["space_id"].(string); ok {
		acc.SpaceID = v
	}
	if v, ok := raw["space_name"].(string); ok {
		acc.SpaceName = v
	}
	if v, ok := raw["space_view_id"].(string); ok {
		acc.SpaceViewID = v
	}
	if v, ok := raw["browser_id"].(string); ok {
		acc.BrowserID = v
	}
	if v, ok := raw["device_id"].(string); ok {
		acc.DeviceID = v
	}
	if v, ok := raw["client_version"].(string); ok {
		acc.ClientVersion = v
	}
	if v, ok := raw["user_agent"].(string); ok {
		acc.UserAgent = v
	}
	if v, ok := raw["timezone"].(string); ok {
		acc.Timezone = v
	}
	if v, ok := raw["default_model"].(string); ok {
		acc.DefaultModel = v
	}
	for k, v := range raw {
		if !known[k] {
			acc.Extras[k] = v
		}
	}
	acc.applyDefaults()
	return acc, nil
}

func Save(acc *NotionAccount, path string) error {
	acc.applyDefaults()
	data := map[string]any{
		"token_v2":       acc.TokenV2,
		"full_cookie":    acc.FullCookie,
		"user_id":        acc.UserID,
		"user_name":      acc.UserName,
		"user_email":     acc.UserEmail,
		"space_id":       acc.SpaceID,
		"space_name":     acc.SpaceName,
		"space_view_id":  acc.SpaceViewID,
		"browser_id":     acc.BrowserID,
		"device_id":      acc.DeviceID,
		"client_version": acc.ClientVersion,
		"user_agent":     acc.UserAgent,
		"timezone":       acc.Timezone,
		"default_model":  acc.DefaultModel,
	}
	for k, v := range acc.Extras {
		data[k] = v
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}