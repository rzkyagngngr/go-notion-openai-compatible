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
	headers := client.BuildHeaders(acc, "")

	b, _ := json.Marshal(body)
	fmt.Printf("request_bytes=%d space_id=%s user_id=%s user_name=%q\n", len(b), acc.SpaceID, acc.UserID, acc.UserName)

	httpClient, err := notionhttp.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer httpClient.Close()

	url := config.DefaultBaseURL + "/runInferenceTranscript"
	reader, status, respBody, err := httpClient.PostStream(url, body, headers)
	if err != nil {
		log.Fatal(err)
	}
	if status != 200 {
		log.Fatalf("status=%d body=%q", status, respBody)
	}
	defer reader.Close()

	raw, err := io.ReadAll(reader)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("response_status=%d response_bytes=%d\n", status, len(raw))
	fmt.Printf("response_text=%q\n", string(raw))
	if len(raw) > 0 && len(raw) <= 64 {
		fmt.Printf("response_hex=%s\n", hex.EncodeToString(raw))
	}
}