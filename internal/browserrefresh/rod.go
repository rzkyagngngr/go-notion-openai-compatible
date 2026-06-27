package browserrefresh

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"github.com/mughu-id/notionchat/internal/account"
)

type rodRefresher struct {
	cfg Config
}

func (r *rodRefresher) Mode() string  { return r.cfg.Mode }
func (r *rodRefresher) Enabled() bool { return true }

func (r *rodRefresher) Ready(ctx context.Context, spaceID string) (bool, error) {
	unlock := acquireLock()
	defer unlock()
	cookie, loggedIn, err := r.extract(ctx, spaceID, false)
	if err != nil {
		return false, err
	}
	return loggedIn && cookie != "", nil
}

func (r *rodRefresher) ExtractSession(ctx context.Context, spaceID string) (string, bool, error) {
	unlock := acquireLock()
	defer unlock()
	return r.extract(ctx, spaceID, true)
}

func (r *rodRefresher) SeedProfile(ctx context.Context, cookieHeader, spaceID string) error {
	params := headerToCookieParams(cookieHeader)
	if len(params) == 0 {
		return nil
	}
	unlock := acquireLock()
	defer unlock()

	timeout := time.Duration(r.cfg.TimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	browser, cleanup, err := r.connect(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return err
	}
	if err := page.Navigate("https://www.notion.so"); err != nil {
		return err
	}
	_ = page.WaitLoad()
	if err := page.SetCookies(params); err != nil {
		return err
	}
	target := r.cfg.LoginURL
	if u := workspaceURL(spaceID); u != "" {
		target = u
	}
	if err := page.Navigate(target); err != nil {
		return err
	}
	_ = page.WaitLoad()
	time.Sleep(2 * time.Second)
	log.Printf("browserrefresh: seeded headless profile with token_v2 (%s)", MaskToken(account.ParseBrowserCookie(cookieHeader)["token_v2"]))
	return nil
}

func (r *rodRefresher) extract(ctx context.Context, spaceID string, navigate bool) (string, bool, error) {
	timeout := time.Duration(r.cfg.TimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	browser, cleanup, err := r.connect(ctx)
	if err != nil {
		return "", false, err
	}
	defer cleanup()

	if navigate {
		target := r.cfg.LoginURL
		if u := workspaceURL(spaceID); u != "" {
			target = u
		}
		page, err := browser.Page(proto.TargetCreateTarget{URL: target})
		if err != nil {
			return "", false, err
		}
		_ = page.WaitLoad()
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			cookies, err := r.collectCookies(browser)
			if err != nil {
				return "", false, err
			}
			if hasToken(cookies) {
				break
			}
			select {
			case <-ctx.Done():
				return "", false, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}

	cookies, err := r.collectCookies(browser)
	if err != nil {
		return "", false, err
	}
	header := cookiesToHeader(cookies)
	return header, header != "", nil
}

func (r *rodRefresher) collectCookies(browser *rod.Browser) ([]*proto.NetworkCookie, error) {
	var sets [][]*proto.NetworkCookie
	for _, origin := range cookieDomains {
		cookies, err := browser.GetCookies()
		if err != nil {
			continue
		}
		filtered := make([]*proto.NetworkCookie, 0)
		for _, c := range cookies {
			if c == nil {
				continue
			}
			if originContains(origin, c.Domain) {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			sets = append(sets, filtered)
		}
	}
	all, err := browser.GetCookies()
	if err != nil {
		return nil, err
	}
	sets = append(sets, all)
	return mergeCookieSets(sets...), nil
}

func originContains(origin, domain string) bool {
	domain = trimDot(domain)
	return containsFold(origin, domain)
}

func trimDot(s string) string {
	for len(s) > 0 && s[0] == '.' {
		s = s[1:]
	}
	return s
}

func containsFold(haystack, needle string) bool {
	return indexFold(haystack, needle) >= 0
}

func indexFold(s, sub string) int {
	ls, lsub := len(s), len(sub)
	for i := 0; i+lsub <= ls; i++ {
		if equalFold(s[i:i+lsub], sub) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func (r *rodRefresher) connect(ctx context.Context) (*rod.Browser, func(), error) {
	if r.cfg.Mode == ModeRemote {
		browser := rod.New().Context(ctx).ControlURL(r.cfg.CDPURL)
		if err := browser.Connect(); err != nil {
			return nil, nil, err
		}
		return browser, func() { _ = browser.Close() }, nil
	}

	if err := os.MkdirAll(r.cfg.ProfileDir, 0o700); err != nil {
		return nil, nil, err
	}
	clearProfileLocks(r.cfg.ProfileDir)

	l := launcher.New().
		Headless(true).
		UserDataDir(r.cfg.ProfileDir).
		Leakless(false).
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("no-first-run").
		Set("no-default-browser-check")
	if r.cfg.ChromiumPath != "" {
		l = l.Bin(r.cfg.ChromiumPath)
	}
	if r.cfg.NoSandbox {
		l = l.NoSandbox(true)
	}
	log.Printf("browserrefresh: launching headless chromium profile=%s", filepath.Base(r.cfg.ProfileDir))
	url, err := l.Launch()
	if err != nil {
		return nil, nil, err
	}
	browser := rod.New().Context(ctx).ControlURL(url)
	if err := browser.Connect(); err != nil {
		return nil, nil, err
	}
	return browser, func() { _ = browser.Close() }, nil
}