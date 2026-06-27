package credentials

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/mughu-id/notionchat/internal/sessionrefresh"
)

func (s *Store) StartBackgroundRefresh(stop <-chan struct{}) {
	interval := refreshInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Credential auto-refresh every %s (NOTION_COOKIE / %s / Notion Set-Cookie)",
		interval, injectFilePath())

	run := func(reason string, probe bool) {
		changed, err := s.RefreshAll()
		if err != nil {
			log.Printf("background refresh (%s): %v", reason, err)
			return
		}
		if changed {
			log.Printf("background refresh (%s): credentials updated", reason)
			probe = true
		}
		if !probe {
			return
		}
		acc, err := s.GetAccount()
		if err != nil || acc == nil {
			return
		}
		if !sessionrefresh.ProbeInference(acc) {
			log.Printf("background refresh: inference probe failed — paste fresh token_v2 at / or data/inject_cookie.txt")
		}
	}

	run("startup", true)
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			run("interval", false)
		}
	}
}

func refreshInterval() time.Duration {
	raw := os.Getenv("NOTION_REFRESH_INTERVAL_SEC")
	if raw == "" {
		return 10 * time.Minute
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec < 60 {
		return 10 * time.Minute
	}
	return time.Duration(sec) * time.Second
}

func injectFilePath() string {
	if p := os.Getenv("NOTION_COOKIE_FILE"); p != "" {
		return p
	}
	return "data/inject_cookie.txt"
}