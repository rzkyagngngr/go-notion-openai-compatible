package credentials

import (
	"context"
	"log"
	"time"

	"github.com/mughu-id/notionchat/internal/account"
)

// SeedBrowserProfile copies the current HTTP session into the server headless Chromium profile.
func (s *Store) SeedBrowserProfile() {
	if !s.browserEnabled() {
		return
	}
	acc, err := s.GetAccount()
	if err != nil || acc == nil || acc.SpaceID == "" {
		return
	}
	cookie := account.BuildCookieHeader(acc)
	if cookie == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.refresher.SeedProfile(ctx, cookie, acc.SpaceID); err != nil {
		log.Printf("browser profile seed: %v", err)
		return
	}
	log.Printf("browser profile seeded for workspace %s — server headless refresh enabled", acc.SpaceID)
	s.invalidateProfileReadyCache()
}

func (s *Store) StartBrowserProfileSeed() {
	go s.SeedBrowserProfile()
}