package credentials

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mughu-id/notionchat/internal/account"
	"github.com/mughu-id/notionchat/internal/bootstrap"
	"github.com/mughu-id/notionchat/internal/browserrefresh"
	"github.com/mughu-id/notionchat/internal/models"
	"github.com/mughu-id/notionchat/internal/notionhttp"
	"github.com/mughu-id/notionchat/internal/sessionrefresh"
)

func (s *Store) RefreshFromEnv() (bool, error) {
	cookie := strings.TrimSpace(os.Getenv("NOTION_COOKIE"))
	if cookie == "" || !strings.Contains(cookie, "token_v2=") {
		return false, nil
	}
	return s.applyExternalCookie(cookie, "env")
}

func (s *Store) RefreshFromInjectFile() (bool, error) {
	if !injectFileAllowed() {
		return false, nil
	}
	path := strings.TrimSpace(os.Getenv("NOTION_COOKIE_FILE"))
	if path == "" {
		path = "data/inject_cookie.txt"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil
	}
	cookie := strings.TrimSpace(string(data))
	if cookie == "" || !strings.Contains(cookie, "token_v2=") {
		return false, nil
	}
	changed, err := s.applyExternalCookie(cookie, "inject_file")
	if err != nil || !changed {
		return changed, err
	}
	_ = os.WriteFile(path, []byte{}, 0o600)
	return true, nil
}

func injectFileAllowed() bool {
	v := strings.TrimSpace(os.Getenv("NOTION_ALLOW_INJECT_FILE"))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func (s *Store) RotateFromNotionAPI() (bool, error) {
	s.mu.RLock()
	acc := s.account
	s.mu.RUnlock()
	if acc == nil || acc.SpaceID == "" {
		return false, nil
	}
	merged, changed, err := sessionrefresh.RotateViaLoadUserContent(acc)
	if err != nil || !changed {
		return false, err
	}
	return s.applyExternalCookie(merged, "http_rotate")
}

func (s *Store) RefreshAll() (bool, error) {
	var httpChanged bool
	for _, fn := range []func() (bool, error){
		s.RefreshFromEnv,
		s.RefreshFromInjectFile,
		s.RotateFromNotionAPI,
	} {
		ok, err := fn()
		if err != nil {
			return httpChanged, err
		}
		if ok {
			httpChanged = true
		}
	}

	acc, _ := s.GetAccount()
	if acc != nil {
		if sessionrefresh.ProbeInference(acc) {
			return httpChanged, nil
		}
	} else if !s.browserEnabled() {
		return httpChanged, nil
	}

	if !s.browserEnabled() {
		return httpChanged, nil
	}

	browserChanged, err := s.RefreshFromBrowser(false)
	return httpChanged || browserChanged, err
}

func (s *Store) RefreshFromBrowser(force bool) (bool, error) {
	if !s.browserEnabled() {
		return false, nil
	}
	if !force && !s.browserDue() {
		return false, nil
	}

	acc, _ := s.GetAccount()
	spaceID := ""
	if acc != nil {
		spaceID = acc.SpaceID
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.browserCfg.TimeoutSec)*time.Second)
	defer cancel()

	cookie, loggedIn, err := s.refresher.ExtractSession(ctx, spaceID)
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	s.lastBrowserRefreshAt = now
	s.mu.Unlock()
	_ = s.persist()

	if err != nil {
		log.Printf("browserrefresh: extract failed: %v", err)
		return false, err
	}
	if !loggedIn || cookie == "" {
		log.Printf("browserrefresh: profile not logged in")
		return false, nil
	}

	changed, err := s.applyExternalCookie(cookie, "browser")
	if err != nil {
		return false, err
	}
	if acc, err := s.GetAccount(); err == nil && acc != nil {
		if sessionrefresh.ProbeInference(acc) {
			log.Printf("browserrefresh: token recovered (%s)", browserrefresh.MaskToken(account.ParseBrowserCookie(cookie)["token_v2"]))
		} else {
			log.Printf("browserrefresh: cookie updated but inference still stale")
		}
	}
	return changed, nil
}

func (s *Store) browserEnabled() bool {
	return s.refresher != nil && s.refresher.Enabled()
}

func (s *Store) browserDue() bool {
	s.mu.RLock()
	last := s.lastBrowserRefreshAt
	s.mu.RUnlock()
	if last == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, last)
	if err != nil {
		return true
	}
	interval := time.Duration(s.browserCfg.MinIntervalSec) * time.Second
	return time.Since(t) >= interval
}

func (s *Store) SessionHealthy() bool {
	acc, err := s.GetAccount()
	if err != nil || acc == nil {
		return false
	}
	return sessionrefresh.ProbeInference(acc)
}

func (s *Store) applyExternalCookie(cookie, source string) (bool, error) {
	input := normalizeInput(SessionInput{Cookie: cookie, SpaceName: s.currentSpaceName()})
	newToken := input.TokenV2
	if newToken == "" {
		return false, nil
	}

	s.mu.RLock()
	oldToken := s.raw.TokenV2
	acc := s.account
	s.mu.RUnlock()
	if oldToken == "" && acc != nil {
		oldToken = acc.TokenV2
	}
	if oldToken == newToken {
		return false, nil
	}

	if identityChanged(acc, input) {
		log.Printf("Credential refresh (%s): identity changed, full bootstrap", source)
		_, err := s.Connect(input)
		if err == nil {
			s.setCredentialSource(normalizeSource(source))
		}
		return true, err // Connect already seeds browser profile
	}

	log.Printf("Credential refresh (%s): token_v2 updated", source)
	if err := s.patchToken(input, cookie); err != nil {
		return false, err
	}
	s.setCredentialSource(normalizeSource(source))
	s.StartBrowserProfileSeed()
	return true, nil
}

func (s *Store) setCredentialSource(source string) {
	s.mu.Lock()
	s.credentialSource = source
	s.mu.Unlock()
	_ = s.persist()
}

func normalizeSource(source string) string {
	switch source {
	case "env":
		return "env"
	case "inject_file":
		return "inject_file"
	case "http_rotate":
		return "http_rotate"
	case "browser":
		return "browser"
	case "API inject":
		return "manual"
	default:
		return source
	}
}

func (s *Store) patchToken(input SessionInput, cookie string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account == nil {
		return nil
	}
	parsed := account.ParseBrowserCookie(cookie)
	token := parsed["token_v2"]
	if token == "" {
		token = input.TokenV2
	}
	s.account = refreshAccountCookies(s.account, cookie)
	s.account.TokenV2 = token
	s.account.FullCookie = account.BuildCookieHeader(s.account)
	s.raw.TokenV2 = token
	s.raw.Cookie = cookie
	if v := parsed["notion_browser_id"]; v != "" {
		s.raw.NotionBrowserID = v
	}
	if v := parsed["notion_user_id"]; v != "" {
		s.raw.NotionUserID = v
	}
	if v := parsed["device_id"]; v != "" {
		s.raw.DeviceID = v
	}
	s.updatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := account.Save(s.account, s.accountPath); err != nil {
		return err
	}
	models.ClearCache()
	return s.persist()
}

func (s *Store) currentSpaceName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.raw.SpaceName != "" {
		return s.raw.SpaceName
	}
	if s.account != nil {
		return s.account.SpaceName
	}
	return ""
}

func identityChanged(acc *account.NotionAccount, input SessionInput) bool {
	if acc == nil {
		return true
	}
	parsed := account.ParseBrowserCookie(buildCookie(input))
	if uid := parsed["notion_user_id"]; uid != "" && acc.UserID != "" && uid != acc.UserID {
		return true
	}
	if acc.SpaceID == "" || acc.SpaceViewID == "" {
		return true
	}
	return false
}

func (s *Store) ApplyInjectedCookie(cookie string) (bool, error) {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return false, nil
	}
	changed, err := s.applyExternalCookie(cookie, "manual")
	return changed, err
}

func CookieChanged(oldCookie, newCookie string) bool {
	return notionhttp.CookieValue(oldCookie, "token_v2") != notionhttp.CookieValue(newCookie, "token_v2")
}

func (s *Store) RebootstrapIfNeeded(cookie string) (*account.NotionAccount, error) {
	var spaceName *string
	if sn := s.currentSpaceName(); sn != "" {
		spaceName = &sn
	}
	return bootstrap.FromCookie(cookie, bootstrap.Options{
		SpaceName:   spaceName,
		AccountPath: s.accountPath,
	})
}