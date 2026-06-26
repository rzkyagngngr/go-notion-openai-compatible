package client

import (
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/mughu-id/notionchat/internal/account"
	"github.com/mughu-id/notionchat/internal/errors"
	"github.com/mughu-id/notionchat/internal/models"
	"github.com/mughu-id/notionchat/internal/ndjson"
	"github.com/mughu-id/notionchat/internal/notionhttp"
	"github.com/mughu-id/notionchat/internal/thread"
	"github.com/mughu-id/notionchat/internal/tools"
	"github.com/mughu-id/notionchat/internal/transcript"
)

type ChatResult struct {
	Text         string
	ThreadID     string
	Model        string
	ToolCalls    []map[string]any
	InputTokens  int
	OutputTokens int
}

type NotionAIClient struct {
	Account        *account.NotionAccount
	BaseURL        string
	ThreadStateDir string
}

func BuildHeaders(acc *account.NotionAccount, accept string) map[string]string {
	if accept == "" {
		accept = "application/x-ndjson"
	}
	return map[string]string{
		"accept":                      accept,
		"accept-encoding":             "identity",
		"accept-language":             "en-US,en;q=0.9",
		"content-type":                "application/json",
		"notion-audit-log-platform":   "web",
		"notion-client-version":       acc.ClientVersion,
		"origin":                      "https://app.notion.com",
		"referer":                     "https://app.notion.com/ai",
		"user-agent":                  acc.UserAgent,
		"x-notion-active-user-header": acc.UserID,
		"x-notion-space-id":           acc.SpaceID,
		"sec-ch-ua":                   `"Google Chrome";v="149", "Chromium";v="149", "Not)A;Brand";v="24"`,
		"sec-ch-ua-mobile":            "?0",
		"sec-ch-ua-platform":          `"Windows"`,
		"sec-fetch-dest":              "empty",
		"sec-fetch-mode":              "cors",
		"sec-fetch-site":              "same-origin",
		"cookie":                      account.BuildCookieHeader(acc),
	}
}

type prepareResult struct {
	body            map[string]any
	headers         map[string]string
	activeThreadID  string
	notionModel     string
	saveState       func()
}

func (c *NotionAIClient) prepare(prompt, system, model, threadID, latestUser string, ideAgentMode bool) (*prepareResult, error) {
	// Agent mode: send transcript + inline instructions, but never OpenAI tool schemas
	// (those are stripped in PrepareChatInput before reaching this layer).
	joined := prompt
	if system != "" {
		joined = system + "\n\n" + prompt
	}
	if strings.TrimSpace(joined) == "" {
		return nil, errors.New("Empty prompt", 400)
	}
	if latestUser == "" {
		latestUser = prompt
		if idx := strings.LastIndex(prompt, "User: "); idx >= 0 {
			latestUser = strings.TrimSpace(prompt[idx+len("User: "):])
		}
	}

	notionModel := models.ResolveModel(model, c.Account.DefaultModel, models.GetCachedAliasMap())
	log.Printf("Notion model: request=%q -> %q", model, notionModel)

	reuseThreadID := threadID
	var prior *thread.State
	if threadID != "" {
		if s, err := thread.Load(threadID, c.ThreadStateDir); err == nil {
			prior = s
			if prior.NotionModel != notionModel {
				log.Printf("Model changed on thread %s — starting new Notion thread", threadID)
				reuseThreadID = ""
			}
		}
	}

	var (
		transcriptData []map[string]any
		activeThreadID string
		createThread   bool
		isPartial      bool
		saveState      func()
	)

	if reuseThreadID != "" && prior != nil {
		updatedIDs := append(append([]string{}, prior.UpdatedConfigIDs...), transcript.NewUUID())
		transcriptData = transcript.BuildPartialTranscript(
			c.Account, joined, notionModel,
			prior.ConfigID, prior.ContextID, prior.OriginalDatetime,
			updatedIDs, ideAgentMode,
		)
		activeThreadID = reuseThreadID
		createThread = false
		isPartial = true
		log.Printf("Continuing Notion thread %s (partial, user_msg=%q)", activeThreadID, truncateForLog(latestUser, 80))
		saveState = func() {
			prior.UpdatedConfigIDs = updatedIDs
			prior.NotionModel = notionModel
			prior.LastActivityISO = transcript.NowISO(c.Account.Timezone)
			_ = thread.Save(prior, c.ThreadStateDir)
		}
	} else {
		configID := transcript.NewUUID()
		contextID := transcript.NewUUID()
		firstDT := transcript.NowISO(c.Account.Timezone)
		transcriptData = transcript.BuildFullTranscript(c.Account, joined, notionModel, configID, contextID, firstDT, ideAgentMode)
		activeThreadID = transcript.NewUUID()
		createThread = true
		isPartial = false
		log.Printf("Starting new Notion thread %s", activeThreadID)
		saveState = func() {
			_ = thread.Save(&thread.State{
				ThreadID: activeThreadID, ConfigID: configID, ContextID: contextID,
				OriginalDatetime: firstDT, NotionModel: notionModel,
			}, c.ThreadStateDir)
		}
	}

	cookie := account.BuildCookieHeader(c.Account)
	if c.Account.UserID != "" && !strings.Contains(cookie, "notion_user_id="+c.Account.UserID) {
		log.Printf("Notion cookie missing user_id=%q — inference may return empty", c.Account.UserID)
	}

	body := transcript.BuildInferenceRequest(c.Account, transcriptData, activeThreadID, createThread, isPartial, "")
	return &prepareResult{
		body: body, headers: BuildHeaders(c.Account, ""),
		activeThreadID: activeThreadID, notionModel: notionModel, saveState: saveState,
	}, nil
}

func (c *NotionAIClient) raiseHTTP(status int, body string) error {
	snippet := body
	if len(snippet) > 500 {
		snippet = snippet[:500]
	}
	if status == 401 || status == 403 {
		return errors.New(fmt.Sprintf("Notion auth failed (%d). Refresh token_v2 cookie. %q", status, snippet), 401)
	}
	return errors.New(fmt.Sprintf("Notion API %d: %q", status, snippet), 502)
}

func emptyResponseMessage(result *ndjson.ParseResult, threadID string) string {
	if result.LineCount == 0 {
		return "Notion returned no stream data. Check space_id and refresh your cookie via /config."
	}
	for _, sample := range result.SampleLines {
		if strings.TrimSpace(sample) == "[]" {
			return "Notion returned an empty inference stream. Reconnect at / — your cookie may be missing notion_user_id or token_v2 may have expired."
		}
	}
	events := make([]string, 0, len(result.EventTypeCounts))
	for k, v := range result.EventTypeCounts {
		events = append(events, fmt.Sprintf("%s=%d", k, v))
	}
	return fmt.Sprintf("Notion returned empty assistant text (thread=%s, events: %s). AI credits may be exhausted.", threadID, strings.Join(events, ", "))
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func shouldReleaseStreamBuffer(text string, ideAgentMode bool) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if ideAgentMode && len(trimmed) >= 8 {
		return true
	}
	return len(trimmed) >= 48 ||
		strings.Contains(text, "\n") ||
		strings.Contains(text, "\n#") ||
		strings.HasPrefix(trimmed, "#")
}

func (c *NotionAIClient) runInference(
	prompt, system, model, threadID, latestUser string,
	ideAgentMode, toolsActive bool,
	clientTools []map[string]any,
	onDelta func(string),
) (*ChatResult, error) {
	prep, err := c.prepare(prompt, system, model, threadID, latestUser, ideAgentMode)
	if err != nil {
		return nil, err
	}

	httpClient, err := notionhttp.NewClient()
	if err != nil {
		return nil, err
	}
	defer httpClient.Close()

	url := c.BaseURL + "/runInferenceTranscript"
	bodyReader, status, respBody, err := httpClient.PostStream(url, prep.body, prep.headers)
	if err != nil {
		return nil, errors.New("Notion transport error: "+err.Error(), 502)
	}
	if status != 200 {
		return nil, c.raiseHTTP(status, respBody)
	}
	defer bodyReader.Close()

	parser := ndjson.NewStreamParser()
	lastEmitted := ""
	hasReleased := false

	err = notionhttp.ReadLines(bodyReader, func(line string) error {
		if err := parser.FeedLine(line); err != nil {
			return err
		}
		if onDelta == nil {
			return nil
		}
		text := parser.Text()
		if !hasReleased {
			if !shouldReleaseStreamBuffer(text, ideAgentMode) {
				return nil
			}
			hasReleased = true
		}
		cleaned := ndjson.CleanNotionOutputText(text)
		if cleaned != "" && len(cleaned) > len(lastEmitted) {
			delta := cleaned[len(lastEmitted):]
			onDelta(delta)
			lastEmitted = cleaned
		}
		return nil
	})
	if err != nil {
		if e, ok := err.(*errors.NotionChatError); ok {
			return nil, e
		}
		return nil, errors.New("Notion transport error: "+err.Error(), 502)
	}

	if onDelta != nil && !hasReleased && parser.Text() != "" {
		cleaned := ndjson.CleanNotionOutputText(parser.Text())
		if cleaned != "" && len(cleaned) > len(lastEmitted) {
			onDelta(cleaned[len(lastEmitted):])
		}
	}

	result := parser.Finalize()
	rawText := result.Text
	if rawText == "" && result.Thinking != "" {
		rawText = result.Thinking
	}
	content, toolCalls := tools.MergeToolCalls(rawText, result.ToolCalls, toolsActive, clientTools, prompt, ideAgentMode)
	if content == "" && len(toolCalls) == 0 {
		log.Printf("Notion inference empty (%s)", parser.DebugSummary())
		if ideAgentMode {
			return &ChatResult{
				Text: "", ThreadID: prep.activeThreadID,
				Model: chooseStr(result.NotionModel, prep.notionModel),
				InputTokens: result.InputTokens, OutputTokens: result.OutputTokens,
			}, nil
		}
		return nil, errors.New(emptyResponseMessage(result, prep.activeThreadID), 502)
	}
	prep.saveState()

	outText := content
	if ideAgentMode {
		outText = rawText
	}
	return &ChatResult{
		Text: outText, ThreadID: prep.activeThreadID,
		Model: chooseStr(result.NotionModel, prep.notionModel),
		ToolCalls: toolCalls, InputTokens: result.InputTokens, OutputTokens: result.OutputTokens,
	}, nil
}

func (c *NotionAIClient) Complete(prompt, system, model, threadID, latestUser string, ideAgentMode, toolsActive bool, clientTools []map[string]any) (*ChatResult, error) {
	return c.runInference(prompt, system, model, threadID, latestUser, ideAgentMode, toolsActive, clientTools, nil)
}

type StreamHandle struct {
	Deltas   <-chan string
	ThreadID string
	Finalize func() (*ChatResult, error)
}

func (c *NotionAIClient) StreamDeltas(prompt, system, model, threadID, latestUser string, ideAgentMode, toolsActive bool, clientTools []map[string]any) (*StreamHandle, error) {
	prep, err := c.prepare(prompt, system, model, threadID, latestUser, ideAgentMode)
	if err != nil {
		return nil, err
	}

	httpClient, err := notionhttp.NewClient()
	if err != nil {
		return nil, err
	}

	url := c.BaseURL + "/runInferenceTranscript"
	bodyReader, status, respBody, err := httpClient.PostStream(url, prep.body, prep.headers)
	if err != nil {
		httpClient.Close()
		return nil, errors.New("Notion transport error: "+err.Error(), 502)
	}
	if status != 200 {
		bodyReader.Close()
		httpClient.Close()
		return nil, c.raiseHTTP(status, respBody)
	}

	parser := ndjson.NewStreamParser()
	ch := make(chan string, 32)
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer close(ch)
		defer bodyReader.Close()
		defer httpClient.Close()
		lastEmitted := ""
		hasReleased := false
		_ = notionhttp.ReadLines(bodyReader, func(line string) error {
			if err := parser.FeedLine(line); err != nil {
				return err
			}
			text := parser.Text()
			if !hasReleased {
				if !shouldReleaseStreamBuffer(text, ideAgentMode) {
					return nil
				}
				hasReleased = true
			}
			cleaned := ndjson.CleanNotionOutputText(text)
			if cleaned != "" && len(cleaned) > len(lastEmitted) {
				ch <- cleaned[len(lastEmitted):]
				lastEmitted = cleaned
			}
			return nil
		})
		if !hasReleased && parser.Text() != "" {
			cleaned := ndjson.CleanNotionOutputText(parser.Text())
			if cleaned != "" && len(cleaned) > len(lastEmitted) {
				ch <- cleaned[len(lastEmitted):]
			}
		}
	}()

	finalize := func() (*ChatResult, error) {
		wg.Wait()
		result := parser.Finalize()
		rawText := result.Text
		if rawText == "" && result.Thinking != "" {
			rawText = result.Thinking
		}
		content, toolCalls := tools.MergeToolCalls(rawText, result.ToolCalls, toolsActive, clientTools, prompt, ideAgentMode)
		if content == "" && len(toolCalls) == 0 {
			if ideAgentMode {
				return &ChatResult{
					Text: "", ThreadID: prep.activeThreadID,
					Model: chooseStr(result.NotionModel, prep.notionModel),
					InputTokens: result.InputTokens, OutputTokens: result.OutputTokens,
				}, nil
			}
			return nil, errors.New(emptyResponseMessage(result, prep.activeThreadID), 502)
		}
		prep.saveState()
		outText := content
		if ideAgentMode {
			outText = rawText
		}
		return &ChatResult{
			Text: outText, ThreadID: prep.activeThreadID,
			Model: chooseStr(result.NotionModel, prep.notionModel),
			ToolCalls: toolCalls, InputTokens: result.InputTokens, OutputTokens: result.OutputTokens,
		}, nil
	}

	return &StreamHandle{Deltas: ch, ThreadID: prep.activeThreadID, Finalize: finalize}, nil
}

func (c *NotionAIClient) FetchAvailableModels() (map[string]any, error) {
	httpClient, err := notionhttp.NewClient()
	if err != nil {
		return nil, err
	}
	defer httpClient.Close()
	url := c.BaseURL + "/getAvailableModels"
	headers := BuildHeaders(c.Account, "application/json")
	data, status, _, err := httpClient.PostJSON(url, map[string]any{"spaceId": c.Account.SpaceID}, headers)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, errors.New(fmt.Sprintf("getAvailableModels failed: %d", status), 502)
	}
	return data, nil
}

func chooseStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

var _ = io.EOF