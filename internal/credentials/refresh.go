package credentials

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/mughu-id/notionchat/internal/account"
	"github.com/mughu-id/notionchat/internal/bootstrap"
	"github.com/mughu-id/notionchat/internal/models"
	"github.com/mughu-id/notionchat/internal/notionhttp"
	"github.com/mughu-id/notionchat/internal/sessionrefresh"
)

func (s *Store) RefreshFromEnv() (bool, error) {
	cookie := strings.TrimSpace(os.Getenv("NOTION_COOKIE"))
	if cookie == "" || !strings.Contains(cookie, "token_v2=") {
		return false, nil
	}
	return s.applyExternalCookie(cookie, "env NOTION_COOKIE")
}

func (s *Store) RefreshFromInjectFile() (bool, error) {
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
	changed, err := s.applyExternalCookie(cookie, "inject file")
	if err != nil || !changed {
		return changed, err
	}
	_ = os.WriteFile(path, []byte{}, 0o600)
	return true, nil
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
	return s.applyExternalCookie(merged, "Notion Set-Cookie")
}

func (s *Store) RefreshAll() (bool, error) {
	var changed bool
	for _, fn := range []func() (bool, error){
		s.RefreshFromEnv,
		s.RefreshFromInjectFile,
		s.RotateFromNotionAPI,
	} {
		ok, err := fn()
		if err != nil {
			return changed, err
		}
		if ok {
			changed = true
		}
	}
	return changed, nil
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
		return true, err
	}

	log.Printf("Credential refresh (%s): token_v2 updated", source)
	return true, s.patchToken(input, cookie)
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
	return s.applyExternalCookie(cookie, "API inject")
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