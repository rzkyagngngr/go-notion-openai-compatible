package bootstrap

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mughu-id/notionchat/internal/account"
	"github.com/mughu-id/notionchat/internal/errors"
	"github.com/mughu-id/notionchat/internal/notionhttp"
)

const baseURL = "https://app.notion.com/api/v3"

type Workspace struct {
	SpaceID     string
	SpaceViewID string
	SpaceName   string
	Domain      string
}

type Options struct {
	SpaceName   *string
	AccountPath string
}

func FromCookie(cookie string, opts Options) (*account.NotionAccount, error) {
	parsed := account.ParseBrowserCookie(cookie)
	token := parsed["token_v2"]
	if token == "" {
		return nil, errors.New("Cookie missing token_v2", 400)
	}
	userID := parsed["notion_user_id"]
	browserID := parsed["notion_browser_id"]
	if browserID == "" {
		browserID = uuid.New().String()
	}
	deviceID := parsed["device_id"]
	if deviceID == "" {
		deviceID = uuid.New().String()
	}

	headers := bootstrapHeaders(token, browserID, userID)
	client, err := notionhttp.NewClient()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	body := map[string]any{
		"cursor": map[string]any{"stack": []any{}},
		"limit":  100,
	}
	data, status, respBody, err := client.PostJSON(baseURL+"/loadUserContent", body, headers)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, errors.New(fmt.Sprintf("loadUserContent failed (%d): %q", status, truncate(respBody, 300)), 502)
	}

	acc, err := accountFromLoadUserContent(cookie, data, token, userID, browserID, deviceID, opts.SpaceName)
	if err != nil {
		return nil, err
	}
	path := opts.AccountPath
	if path == "" {
		path = "notion_account.json"
	}
	if err := account.Save(acc, path); err != nil {
		return nil, err
	}
	return acc, nil
}

func bootstrapHeaders(tokenV2, browserID, userID string) map[string]string {
	parts := []string{"notion_browser_id=" + browserID}
	if userID != "" {
		parts = append(parts, "notion_user_id="+userID)
		parts = append(parts, fmt.Sprintf(`notion_users=[%%22%s%%22]`, userID))
	}
	parts = append(parts, "notion_check_cookie_consent=false", "token_v2="+tokenV2)
	return map[string]string{
		"accept":                    "application/json",
		"content-type":              "application/json",
		"notion-audit-log-platform": "web",
		"notion-client-version":     account.DefaultClientVersion,
		"origin":                    "https://app.notion.com",
		"referer":                   "https://app.notion.com/",
		"cookie":                    strings.Join(parts, "; "),
	}
}

func accountFromLoadUserContent(
	cookie string,
	data map[string]any,
	token, userID, browserID, deviceID string,
	spaceName *string,
) (*account.NotionAccount, error) {
	recordMap, _ := data["recordMap"].(map[string]any)
	if recordMap == nil {
		return nil, errors.New("Invalid loadUserContent response", 502)
	}
	if userID == "" {
		if users, ok := recordMap["notion_user"].(map[string]any); ok {
			for uid := range users {
				userID = uid
				break
			}
		}
	}
	if userID == "" {
		return nil, errors.New("Could not determine notion_user_id", 502)
	}

	userName, userEmail := extractUser(recordMap, userID)
	workspaces := extractWorkspaces(recordMap)
	if len(workspaces) == 0 {
		return nil, errors.New("No workspaces found for this account", 502)
	}

	chosen := workspaces[0]
	if spaceName != nil && *spaceName != "" {
		found := false
		for _, w := range workspaces {
			if strings.EqualFold(w.SpaceName, *spaceName) {
				chosen = w
				found = true
				break
			}
		}
		if !found {
			names := make([]string, len(workspaces))
			for i, w := range workspaces {
				names[i] = w.SpaceName
			}
			return nil, errors.New(fmt.Sprintf("Workspace %q not found. Available: %s", *spaceName, strings.Join(names, ", ")), 400)
		}
	} else if len(workspaces) > 1 {
		names := make([]string, len(workspaces))
		for i, w := range workspaces {
			names[i] = w.SpaceName
		}
		return nil, errors.New(fmt.Sprintf(
			"Multiple workspaces found. Set NOTION_SPACE_NAME via /config. Available: %s",
			strings.Join(names, ", "),
		), 400)
	}

	return &account.NotionAccount{
		TokenV2:       token,
		FullCookie:    cookie,
		UserID:        userID,
		UserName:      userName,
		UserEmail:     userEmail,
		SpaceID:       chosen.SpaceID,
		SpaceName:     chosen.SpaceName,
		SpaceViewID:   chosen.SpaceViewID,
		BrowserID:     browserID,
		DeviceID:      deviceID,
		ClientVersion: account.DefaultClientVersion,
	}, nil
}

func extractUser(recordMap map[string]any, userID string) (string, string) {
	users, _ := recordMap["notion_user"].(map[string]any)
	entry, _ := users[userID].(map[string]any)
	value, _ := entry["value"].(map[string]any)
	inner, _ := value["value"].(map[string]any)
	name := ""
	if nameList, ok := inner["name"].([]any); ok && len(nameList) > 0 {
		if first, ok := nameList[0].([]any); ok && len(first) > 0 {
			name, _ = first[0].(string)
		}
	}
	email, _ := inner["email"].(string)
	return name, email
}

func extractWorkspaces(recordMap map[string]any) []Workspace {
	var spaces []Workspace
	spaceMap, _ := recordMap["space"].(map[string]any)
	viewMap, _ := recordMap["space_view"].(map[string]any)
	for viewID, viewEntry := range viewMap {
		entry, _ := viewEntry.(map[string]any)
		value, _ := entry["value"].(map[string]any)
		viewVal, _ := value["value"].(map[string]any)
		spaceID, _ := viewVal["space_id"].(string)
		if spaceID == "" {
			spaceID, _ = viewVal["parent_id"].(string)
		}
		if spaceID == "" {
			continue
		}
		spaceEntry, _ := spaceMap[spaceID].(map[string]any)
		spaceVal, _ := spaceEntry["value"].(map[string]any)
		spaceInner, _ := spaceVal["value"].(map[string]any)
		name := spaceID
		if nameList, ok := spaceInner["name"].([]any); ok && len(nameList) > 0 {
			if first, ok := nameList[0].([]any); ok && len(first) > 0 {
				name, _ = first[0].(string)
			}
		}
		domain, _ := spaceInner["domain"].(string)
		spaces = append(spaces, Workspace{
			SpaceID:     spaceID,
			SpaceViewID: viewID,
			SpaceName:   name,
			Domain:      domain,
		})
	}
	return spaces
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func MarshalDebug(data map[string]any) string {
	b, _ := json.Marshal(data)
	return string(b)
}