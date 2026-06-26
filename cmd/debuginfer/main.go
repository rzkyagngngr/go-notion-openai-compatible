package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mughu-id/notionchat/internal/account"
	"github.com/mughu-id/notionchat/internal/client"
	"github.com/mughu-id/notionchat/internal/config"
	"github.com/mughu-id/notionchat/internal/notionhttp"
	"github.com/mughu-id/notionchat/internal/transcript"
)

func main() {
	path := "data/notion_account.json"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	acc, err := account.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	acc.FullCookie = account.BuildCookieHeader(acc)

	notionModel := acc.DefaultModel
	configID := transcript.NewUUID()
	contextID := transcript.NewUUID()
	now := transcript.NowISO(acc.Timezone)
	transcriptData := transcript.BuildFullTranscript(acc, "halo", notionModel, configID, contextID, now, false)
	threadID := transcript.NewUUID()
	body := transcript.BuildInferenceRequest(acc, transcriptData, threadID, true, false, "")

	b, _ := json.Marshal(body)
	_ = os.WriteFile("/tmp/infer_body.json", b, 0o600)
	fmt.Printf("request_bytes=%d space_id=%s user_id=%s user_name=%q\n", len(b), acc.SpaceID, acc.UserID, acc.UserName)
	fmt.Printf("wrote /tmp/infer_body.json\n")

	httpClient, err := notionhttp.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer httpClient.Close()

	url := config.DefaultBaseURL + "/runInferenceTranscript"

	for _, tc := range []struct {
		label string
		hdr   map[string]string
	}{
		{"with_identity", client.BuildHeaders(acc, "")},
		{"no_identity", func() map[string]string {
			h := client.BuildHeaders(acc, "")
			delete(h, "accept-encoding")
			return h
		}()},
	} {
		fmt.Printf("\n--- %s ---\n", tc.label)
		reader, status, respBody, err := httpClient.PostStream(url, body, tc.hdr)
		if err != nil {
			log.Fatalf("%s transport: %v", tc.label, err)
		}
		if status != 200 {
			fmt.Printf("status=%d body=%q\n", status, respBody)
			reader.Close()
			continue
		}
		raw, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			fmt.Printf("read error: %v\n", err)
			continue
		}
		fmt.Printf("response_status=%d response_bytes=%d\n", status, len(raw))
		fmt.Printf("response_text=%q\n", string(raw))
		if len(raw) > 0 && len(raw) <= 128 {
			fmt.Printf("response_hex=%s\n", hex.EncodeToString(raw))
		}
	}

	fmt.Printf("\n--- net_http ---\n")
	std := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		log.Fatal(err)
	}
	hdr := client.BuildHeaders(acc, "")
	delete(hdr, "accept-encoding")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := std.Do(req)
	if err != nil {
		log.Fatalf("net/http: %v", err)
	}
	defer resp.Body.Close()
	fmt.Printf("status=%d content-encoding=%q\n", resp.StatusCode, resp.Header.Get("Content-Encoding"))
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("net/http read: %v", err)
	}
	fmt.Printf("response_bytes=%d text=%q\n", len(raw), string(raw))
	if len(raw) > 0 && len(raw) <= 128 {
		fmt.Printf("response_hex=%s\n", hex.EncodeToString(raw))
	}
}