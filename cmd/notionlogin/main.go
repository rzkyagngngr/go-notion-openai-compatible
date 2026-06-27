package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mughu-id/notionchat/internal/browserrefresh"
)

func main() {
	profile := flag.String("profile", "data/browser-profile", "Chromium user-data-dir")
	chromium := flag.String("chromium", "", "Chromium binary path")
	headless := flag.Bool("headless", false, "Run headless (use xvfb-run on Linux server)")
	flag.Parse()

	cfg := browserrefresh.Config{
		Mode:         browserrefresh.ModeHeadless,
		ProfileDir:   *profile,
		ChromiumPath: *chromium,
		LoginURL:     "https://www.notion.so/login",
		TimeoutSec:   600,
		NoSandbox:    true,
	}
	if *chromium == "" {
		if _, err := os.Stat("/usr/bin/chromium"); err == nil {
			cfg.ChromiumPath = "/usr/bin/chromium"
		}
	}
	if !*headless {
		os.Setenv("NOTION_BROWSER_HEADLESS", "false")
		cfg.Mode = browserrefresh.ModeRemote
		cfg.CDPURL = "http://127.0.0.1:9222"
		fmt.Println("Start Chrome with --remote-debugging-port=9222, login to Notion, then re-run with --headless")
	}

	refresher := browserrefresh.NewRefresher(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cookie, ok, err := refresher.ExtractSession(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "login harvest failed: %v\n", err)
		os.Exit(1)
	}
	if !ok || cookie == "" {
		fmt.Fprintln(os.Stderr, "Not logged in — complete Notion login in browser window")
		os.Exit(1)
	}
	fmt.Println("Profile seeded — token_v2 present in", *profile)
}