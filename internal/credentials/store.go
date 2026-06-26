package credentials

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mughu-id/notionchat/internal/account"
	"github.com/mughu-id/notionchat/internal/bootstrap"
	"github.com/mughu-id/notionchat/internal/errors"
	"github.com/mughu-id/notionchat/internal/models"
)

const DefaultSessionFile = "data/session.json"

type SessionInput struct {
	NotionBrowserID string `json:"notion_browser_id"`
	TokenV2         string `json:"token_v2"`
	DeviceID        string `json:"device_id,omitempty"`
	NotionUserID    string `json:"notion_user_id,omitempty"`
	Cookie          string `json:"cookie,omitempty"`
	SpaceName       string `json:"space_name,omitempty"`
}

type SessionStatus struct {
	Connected       bool   `json:"connected"`
	NotionBrowserID string `json:"notion_browser_id,omitempty"`
	TokenV2Preview  string `json:"token_v2_preview,omitempty"`
	UserName        string `json:"user_name,omitempty"`
	UserEmail       string `json:"user_email,omitempty"`
	SpaceName       string `json:"space_name,omitempty"`
	SpaceID         string `json:"space_id,omitempty"`
	ConnectedAt     string `json:"connected_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

type persistedSession struct {
	NotionBrowserID string `json:"notion_browser_id"`
	TokenV2         string `json:"token_v2"`
	DeviceID        string `json:"device_id,omitempty"`
	NotionUserID    string `json:"notion_user_id,omitempty"`
	Cookie          string `json:"cookie,omitempty"`
	SpaceName       string `json:"space_name,omitempty"`
	ConnectedAt     string `json:"connected_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

type Store struct {
	mu          sync.RWMutex
	account     *account.NotionAccount
	sessionFile string
	accountPath string
	connectedAt string
	updatedAt   string
	raw         SessionInput
}

func NewStore(sessionFile, accountPath string) *Store {
	if sessionFile == "" {
		sessionFile = DefaultSessionFile
	}
	if accountPath == "" {
		accountPath = "notion_account.json"
	}
	s := &Store{sessionFile: sessionFile, accountPath: accountPath}
	s.loadFromDisk()
	return s
}

func (s *Store) loadFromDisk() {
	data, err := os.ReadFile(s.sessionFile)
	if err != nil {
		return
	}
	var p persistedSession
	if err := json.Unmarshal(data, &p); err != nil {
		return
	}
	if p.TokenV2 == "" && p.Cookie == "" {
		return
	}
	input := SessionInput{
		NotionBrowserID: p.NotionBrowserID,
		TokenV2:         p.TokenV2,
		DeviceID:        p.DeviceID,
		NotionUserID:    p.NotionUserID,
		Cookie:          p.Cookie,
		SpaceName:       p.SpaceName,
	}
	s.mu.Lock()
	s.raw = input
	s.connectedAt = p.ConnectedAt
	s.updatedAt = p.UpdatedAt
	s.mu.Unlock()

	if acc, err := account.Load(s.accountPath); err == nil && acc.SpaceID != "" {
		s.mu.Lock()
		s.account = refreshAccountCookies(acc, buildCookie(input))
		s.mu.Unlock()
	}
}

func (s *Store) Connect(input SessionInput) (*account.NotionAccount, error) {
	input = normalizeInput(input)
	cookie := buildCookie(input)
	if cookie == "" || !strings.Contains(cookie, "token_v2=") {
		return nil, errors.New("token_v2 is required", 400)
	}

	var spaceName *string
	if input.SpaceName != "" {
		spaceName = &input.SpaceName
	}

	acc, err := bootstrap.FromCookie(cookie, bootstrap.Options{
		SpaceName:   spaceName,
		AccountPath: s.accountPath,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	s.account = acc
	s.raw = input
	if s.connectedAt == "" {
		s.connectedAt = now
	}
	s.updatedAt = now
	s.mu.Unlock()

	if err := s.persist(); err != nil {
		return nil, err
	}
	models.ClearCache()
	return acc, nil
}

func (s *Store) Disconnect() error {
	s.mu.Lock()
	s.account = nil
	s.raw = SessionInput{}
	s.connectedAt = ""
	s.updatedAt = time.Now().UTC().Format(time.RFC3339)
	s.mu.Unlock()
	models.ClearCache()
	_ = os.Remove(s.sessionFile)
	return nil
}

func (s *Store) GetAccount() (*account.NotionAccount, error) {
	s.mu.RLock()
	acc := s.account
	s.mu.RUnlock()

	if acc != nil && acc.SpaceID != "" {
		return acc, nil
	}

	if _, err := os.Stat(s.accountPath); err == nil {
		loaded, err := account.Load(s.accountPath)
		if err != nil {
			return nil, err
		}
		if loaded.SpaceID == "" {
			return nil, errors.New("Account missing space_id. Connect via / first.", 500)
		}
		s.mu.Lock()
		s.account = loaded
		s.mu.Unlock()
		return loaded, nil
	}

	return nil, errors.New(
		"Notion session not connected. Open http://127.0.0.1:8787/ and sign in with browser session.",
		401,
	)
}

func (s *Store) Status() SessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st := SessionStatus{
		Connected:       s.account != nil && s.account.SpaceID != "",
		NotionBrowserID: s.raw.NotionBrowserID,
		TokenV2Preview:  maskToken(s.raw.TokenV2),
		ConnectedAt:     s.connectedAt,
		UpdatedAt:       s.updatedAt,
	}
	if s.account != nil {
		st.UserName = s.account.UserName
		st.UserEmail = s.account.UserEmail
		st.SpaceName = s.account.SpaceName
		st.SpaceID = s.account.SpaceID
		if st.NotionBrowserID == "" {
			st.NotionBrowserID = s.account.BrowserID
		}
	}
	return st
}

func (s *Store) persist() error {
	s.mu.RLock()
	p := persistedSession{
		NotionBrowserID: s.raw.NotionBrowserID,
		TokenV2:         s.raw.TokenV2,
		DeviceID:        s.raw.DeviceID,
		NotionUserID:    s.raw.NotionUserID,
		Cookie:          s.raw.Cookie,
		SpaceName:       s.raw.SpaceName,
		ConnectedAt:     s.connectedAt,
		UpdatedAt:       s.updatedAt,
	}
	s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(s.sessionFile), 0o755); err != nil && filepath.Dir(s.sessionFile) != "." {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.sessionFile, append(b, '\n'), 0o600)
}

func normalizeInput(input SessionInput) SessionInput {
	input.NotionBrowserID = strings.TrimSpace(input.NotionBrowserID)
	input.TokenV2 = strings.TrimSpace(input.TokenV2)
	input.Cookie = strings.TrimSpace(input.Cookie)
	input.SpaceName = strings.TrimSpace(input.SpaceName)

	if input.Cookie != "" {
		parsed := account.ParseBrowserCookie(input.Cookie)
		if v := parsed["notion_browser_id"]; v != "" {
			input.NotionBrowserID = v
		}
		if v := parsed["token_v2"]; v != "" {
			input.TokenV2 = v
		}
		if v := parsed["device_id"]; v != "" {
			input.DeviceID = v
		}
		if v := parsed["notion_user_id"]; v != "" {
			input.NotionUserID = v
		}
	}
	return input
}

func buildCookie(input SessionInput) string {
	if input.Cookie != "" {
		return input.Cookie
	}
	if input.TokenV2 == "" {
		return ""
	}
	return account.BuildCookieFromParts(input.NotionBrowserID, input.DeviceID, input.NotionUserID, input.TokenV2)
}

func refreshAccountCookies(acc *account.NotionAccount, cookie string) *account.NotionAccount {
	parsed := account.ParseBrowserCookie(cookie)
	token := parsed["token_v2"]
	if token == "" {
		token = acc.TokenV2
	}
	userID := parsed["notion_user_id"]
	if userID == "" {
		userID = acc.UserID
	}
	browserID := parsed["notion_browser_id"]
	if browserID == "" {
		browserID = acc.BrowserID
	}
	deviceID := parsed["device_id"]
	if deviceID == "" {
		deviceID = acc.DeviceID
	}
	return &account.NotionAccount{
		TokenV2: token, FullCookie: cookie, UserID: userID,
		UserName: acc.UserName, UserEmail: acc.UserEmail,
		SpaceID: acc.SpaceID, SpaceName: acc.SpaceName, SpaceViewID: acc.SpaceViewID,
		BrowserID: browserID, DeviceID: deviceID,
		ClientVersion: acc.ClientVersion, UserAgent: acc.UserAgent,
		Timezone: acc.Timezone, DefaultModel: acc.DefaultModel, Extras: acc.Extras,
	}
}

func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 12 {
		return "••••••••"
	}
	return token[:6] + "••••" + token[len(token)-4:]
}