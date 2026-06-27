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

	log.Printf("Credential auto-refresh every %s (env / inject / HTTP / browser)",
		interval)

	run := func(reason string, mustProbe bool) {
		changed, err := s.RefreshAll()
		if err != nil {
			log.Printf("background refresh (%s): %v", reason, err)
			return
		}
		if changed {
			log.Printf("background refresh (%s): credentials updated", reason)
		}
		if mustProbe || s.ProbeDue() {
			acc, err := s.GetAccount()
			if err == nil && acc != nil && !sessionrefresh.ProbeInference(acc) {
				log.Printf("background refresh: session stale — run notionsync from Windows or POST /api/session/browser-refresh")
			}
			s.MarkProbeDone()
		}
	}

	s.SeedBrowserProfile()
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