package credentials

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mughu-id/notionchat/internal/account"
	"github.com/mughu-id/notionchat/internal/bootstrap"
	"github.com/mughu-id/notionchat/internal/browserrefresh"
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
	Connected            bool   `json:"connected"`
	NotionBrowserID      string `json:"notion_browser_id,omitempty"`
	TokenV2Preview       string `json:"token_v2_preview,omitempty"`
	UserName             string `json:"user_name,omitempty"`
	UserEmail            string `json:"user_email,omitempty"`
	SpaceName            string `json:"space_name,omitempty"`
	SpaceID              string `json:"space_id,omitempty"`
	ConnectedAt          string `json:"connected_at,omitempty"`
	UpdatedAt            string `json:"updated_at,omitempty"`
	BrowserMode          string `json:"browser_mode,omitempty"`
	BrowserProfileReady  bool   `json:"browser_profile_ready"`
	LastBrowserRefreshAt string `json:"last_browser_refresh_at,omitempty"`
	CredentialSource     string `json:"credential_source,omitempty"`
}

type persistedSession struct {
	NotionBrowserID      string `json:"notion_browser_id"`
	TokenV2              string `json:"token_v2"`
	DeviceID             string `json:"device_id,omitempty"`
	NotionUserID         string `json:"notion_user_id,omitempty"`
	Cookie               string `json:"cookie,omitempty"`
	SpaceName            string `json:"space_name,omitempty"`
	ConnectedAt          string `json:"connected_at,omitempty"`
	UpdatedAt            string `json:"updated_at,omitempty"`
	LastBrowserRefreshAt string `json:"last_browser_refresh_at,omitempty"`
	LastProbeAt          string `json:"last_probe_at,omitempty"`
	CredentialSource     string `json:"credential_source,omitempty"`
}

type Store struct {
	mu                    sync.RWMutex
	account               *account.NotionAccount
	sessionFile           string
	accountPath           string
	connectedAt           string
	updatedAt             string
	lastBrowserRefreshAt  string
	lastProbeAt           string
	credentialSource      string
	raw                   SessionInput
	refresher             browserrefresh.Refresher
	browserCfg            browserrefresh.Config
	profileReadyCache     bool
	profileReadyCachedAt  time.Time
}

func NewStore(sessionFile, accountPath string, refresher browserrefresh.Refresher) *Store {
	if sessionFile == "" {
		sessionFile = DefaultSessionFile
	}
	if accountPath == "" {
		accountPath = "notion_account.json"
	}
	if refresher == nil {
		refresher = browserrefresh.NewRefresher(browserrefresh.LoadConfig())
	}
	s := &Store{
		sessionFile: sessionFile,
		accountPath: accountPath,
		refresher:   refresher,
		browserCfg:  browserrefresh.LoadConfig(),
	}
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
	s.lastBrowserRefreshAt = p.LastBrowserRefreshAt
	s.lastProbeAt = p.LastProbeAt
	s.credentialSource = p.CredentialSource
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
	s.credentialSource = "manual"
	s.mu.Unlock()

	if err := s.persist(); err != nil {
		return nil, err
	}
	models.ClearCache()
	s.StartBrowserProfileSeed()
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
		acc.FullCookie = account.BuildCookieHeader(acc)
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
		loaded.FullCookie = account.BuildCookieHeader(loaded)
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
		Connected:            s.account != nil && s.account.SpaceID != "",
		NotionBrowserID:      s.raw.NotionBrowserID,
		TokenV2Preview:       maskToken(s.raw.TokenV2),
		ConnectedAt:          s.connectedAt,
		UpdatedAt:            s.updatedAt,
		LastBrowserRefreshAt: s.lastBrowserRefreshAt,
		CredentialSource:     s.credentialSource,
		BrowserMode:          s.refresher.Mode(),
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
	if s.refresher.Enabled() {
		st.BrowserProfileReady = s.cachedProfileReady(st.SpaceID)
	}
	return st
}

func (s *Store) cachedProfileReady(spaceID string) bool {
	s.mu.RLock()
	if time.Since(s.profileReadyCachedAt) < 60*time.Second {
		ready := s.profileReadyCache
		s.mu.RUnlock()
		return ready
	}
	s.mu.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	ready, _ := s.refresher.Ready(ctx, spaceID)
	cancel()
	s.mu.Lock()
	s.profileReadyCache = ready
	s.profileReadyCachedAt = time.Now()
	s.mu.Unlock()
	return ready
}

func (s *Store) invalidateProfileReadyCache() {
	s.mu.Lock()
	s.profileReadyCachedAt = time.Time{}
	s.mu.Unlock()
}

func (s *Store) persist() error {
	s.mu.RLock()
	p := persistedSession{
		NotionBrowserID:      s.raw.NotionBrowserID,
		TokenV2:              s.raw.TokenV2,
		DeviceID:             s.raw.DeviceID,
		NotionUserID:         s.raw.NotionUserID,
		Cookie:               s.raw.Cookie,
		SpaceName:            s.raw.SpaceName,
		ConnectedAt:          s.connectedAt,
		UpdatedAt:            s.updatedAt,
		LastBrowserRefreshAt: s.lastBrowserRefreshAt,
		LastProbeAt:          s.lastProbeAt,
		CredentialSource:     s.credentialSource,
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

func (s *Store) BrowserRefresher() browserrefresh.Refresher {
	return s.refresher
}

func (s *Store) HealthInfo() map[string]any {
	st := s.Status()
	return map[string]any{
		"status":                "ok",
		"session_connected":     st.Connected,
		"browser_mode":          st.BrowserMode,
		"browser_profile_ready": st.BrowserProfileReady,
		"credential_source":     st.CredentialSource,
	}
}

func (s *Store) MarkProbeDone() {
	s.mu.Lock()
	s.lastProbeAt = time.Now().UTC().Format(time.RFC3339)
	s.mu.Unlock()
	_ = s.persist()
}

func (s *Store) ProbeDue() bool {
	s.mu.RLock()
	last := s.lastProbeAt
	s.mu.RUnlock()
	if last == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, last)
	if err != nil {
		return true
	}
	sec := probeMinIntervalSec()
	return time.Since(t) >= time.Duration(sec)*time.Second
}

func probeMinIntervalSec() int {
	raw := os.Getenv("NOTION_PROBE_MIN_INTERVAL_SEC")
	if raw == "" {
		return 600
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 60 {
		return 600
	}
	return n
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