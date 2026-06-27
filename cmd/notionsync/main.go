package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mughu-id/notionchat/internal/browserrefresh"
	"github.com/mughu-id/notionchat/internal/credentials"
)

func main() {
	cdp := flag.String("cdp", "http://127.0.0.1:9222", "Chrome DevTools URL (http://)")
	url := flag.String("url", "http://127.0.0.1:8787", "NotionChat server base URL")
	space := flag.String("space", "", "Workspace name or space_id")
	apiKey := flag.String("api-key", "", "API key (default NOTIONCHAT_API_KEY or sk-notionchat)")
	flag.Parse()

	key := strings.TrimSpace(*apiKey)
	if key == "" {
		key = os.Getenv("NOTIONCHAT_API_KEY")
	}
	if key == "" {
		key = "sk-notionchat"
	}

	cfg := browserrefresh.Config{
		Mode:       browserrefresh.ModeRemote,
		CDPURL:     strings.TrimSpace(*cdp),
		TimeoutSec: 120,
	}
	refresher := browserrefresh.NewRefresher(cfg)
	if !refresher.Enabled() {
		fmt.Fprintln(os.Stderr, "CDP refresher not enabled — check --cdp URL")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cookie, loggedIn, err := refresher.ExtractSession(ctx, strings.TrimSpace(*space))
	if err != nil {
		fmt.Fprintf(os.Stderr, "CDP harvest failed: %v\n", err)
		os.Exit(1)
	}
	if !loggedIn || cookie == "" {
		fmt.Fprintln(os.Stderr, "token_v2 not found — login to notion.com in Chrome first")
		os.Exit(1)
	}

	parsed := parseCookie(cookie)
	body := map[string]string{
		"notion_browser_id": parsed["notion_browser_id"],
		"token_v2":          parsed["token_v2"],
	}
	if v := strings.TrimSpace(*space); v != "" {
		body["space_name"] = v
	}
	if v := parsed["notion_user_id"]; v != "" {
		body["notion_user_id"] = v
	}
	if v := parsed["device_id"]; v != "" {
		body["device_id"] = v
	}

	base := strings.TrimRight(strings.TrimSpace(*url), "/")
	payload, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, base+"/api/session", bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "request error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "POST /api/session failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "server %d: %s\n", resp.StatusCode, string(data))
		os.Exit(1)
	}

	fmt.Printf("Synced token_v2 (%s) → %s\n", browserrefresh.MaskToken(parsed["token_v2"]), base)
	fmt.Println(string(data))

	// Optional probe via chat (requires API key)
	probeReq, _ := http.NewRequest(http.MethodGet, base+"/api/session", nil)
	probeResp, err := http.DefaultClient.Do(probeReq)
	if err == nil {
		defer probeResp.Body.Close()
		var st credentials.SessionStatus
		_ = json.NewDecoder(probeResp.Body).Decode(&st)
		if st.Connected {
			fmt.Printf("Session OK — workspace %q\n", st.SpaceName)
		}
	}
	_ = key
}

func parseCookie(cookie string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, "=") {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}