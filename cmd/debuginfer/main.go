package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

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
	fmt.Printf("request_bytes=%d space_id=%s user_id=%s user_name=%q\n", len(b), acc.SpaceID, acc.UserID, acc.UserName)

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
}