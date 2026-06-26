package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mughu-id/notionchat/internal/client"
	"github.com/mughu-id/notionchat/internal/config"
	"github.com/mughu-id/notionchat/internal/credentials"
	"github.com/mughu-id/notionchat/internal/errors"
	"github.com/mughu-id/notionchat/internal/models"
	"github.com/mughu-id/notionchat/internal/tools"
	"github.com/mughu-id/notionchat/internal/webui"
)

type Server struct {
	settings       *config.Settings
	credentials    *credentials.Store
	sessionThreads map[string]string
	sessionModels  map[string]string
	mu             sync.Mutex
}

func NewServer(settings *config.Settings, creds *credentials.Store) *Server {
	return &Server{
		settings:       settings,
		credentials:    creds,
		sessionThreads: make(map[string]string),
		sessionModels:  make(map[string]string),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("GET /api/models", s.handleAPIModels)
	mux.HandleFunc("GET /api/info", s.handleAPIInfo)
	webui.New(s.credentials, s.settings).Register(mux)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) verifyKey(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	return token == s.settings.APIKey
}

func (s *Server) getClient() (*client.NotionAIClient, error) {
	settings := config.Get()
	acc, err := s.credentials.GetAccount()
	if err != nil {
		return nil, err
	}
	return &client.NotionAIClient{
		Account: acc, BaseURL: settings.BaseURL, ThreadStateDir: settings.ThreadStateDir,
	}, nil
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if !s.verifyKey(r) {
		writeError(w, "Missing or invalid API key. Use: Authorization: Bearer "+s.settings.APIKey, http.StatusUnauthorized)
		return
	}
	data, err := s.fetchModels()
	if err != nil {
		writeNotionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"object": "list", "data": data})
}

func (s *Server) handleAPIModels(w http.ResponseWriter, r *http.Request) {
	st := s.credentials.Status()
	if !st.Connected {
		writeError(w, "Notion session not connected. Connect via / first.", http.StatusUnauthorized)
		return
	}
	data, err := s.fetchModels()
	if err != nil {
		writeNotionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"object": "list", "data": data})
}

func (s *Server) handleAPIInfo(w http.ResponseWriter, r *http.Request) {
	settings := config.Get()
	writeJSON(w, map[string]any{
		"api_key":  settings.APIKey,
		"base_url": fmt.Sprintf("http://%s:%d/v1", settings.Host, settings.Port),
		"models_url": fmt.Sprintf("http://%s:%d/v1/models", settings.Host, settings.Port),
		"note": "Untuk Cursor/Postman: set Base URL dan API Key di atas. /v1/models butuh header Authorization: Bearer <api_key>",
	})
}

func (s *Server) fetchModels() ([]map[string]any, error) {
	if cached := models.GetCachedOpenAIModels(); cached != nil {
		return cached, nil
	}
	c, err := s.getClient()
	if err != nil {
		return nil, err
	}
	raw, err := c.FetchAvailableModels()
	if err != nil {
		log.Printf("getAvailableModels failed: %v — using fallback", err)
		return models.ListOpenAIModelsFallback(), nil
	}
	data := models.ListOpenAIModelsFromNotion(raw)
	models.CacheOpenAIModels(data, models.ParseAvailableModels(raw))
	return data, nil
}

type chatRequest struct {
	Model      string               `json:"model"`
	Messages   []tools.ChatMessage  `json:"messages"`
	Stream     *bool                `json:"stream"`
	User       string               `json:"user"`
	Tools      []map[string]any     `json:"tools"`
	ToolChoice any                  `json:"tool_choice"`
}

func wantsStream(req *chatRequest, r *http.Request, toolsActive, ideAgent bool) bool {
	if toolsActive || ideAgent {
		return true
	}
	if accept := r.Header.Get("Accept"); strings.Contains(accept, "text/event-stream") {
		return true
	}
	if req.Stream != nil {
		return *req.Stream
	}
	return true
}

func streamFieldLabel(stream *bool) string {
	if stream == nil {
		return "omitted"
	}
	if *stream {
		return "true"
	}
	return "false"
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.verifyKey(r) {
		writeError(w, "Missing or invalid API key", http.StatusUnauthorized)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		req.Model = "notion-ai"
	}

	settings := config.Get()
	system, prompt, toolsActive, ideAgent, normalizedTools, err := tools.PrepareChatInput(req.Messages, req.Tools, req.ToolChoice)
	if err != nil {
		writeNotionError(w, err)
		return
	}

	sessionKey := tools.SessionKeyFromMessages(req.User, s.settings.APIKey, req.Messages)
	latestUser := tools.ExtractLastUserMessage(req.Messages)
	threadID := ""
	if !ideAgent && !toolsActive {
		threadID = s.resolveThreadID(sessionKey, req.Model, req.Messages)
	}
	useStream := wantsStream(&req, r, toolsActive, ideAgent)
	log.Printf(
		"chat stream_field=%s use_stream=%v tools=%v ide_agent=%v session=%q thread=%q msgs=%d latest_user=%q",
		streamFieldLabel(req.Stream), useStream, toolsActive, ideAgent,
		sessionKey, threadID, len(req.Messages), truncate(latestUser, 60),
	)

	c, err := s.getClient()
	if err != nil {
		writeNotionError(w, err)
		return
	}

	s.ensureModelAliases(c, settings)

	if toolsActive {
		if preempt := tools.PreemptiveAgentToolCalls(req.Messages, normalizedTools); len(preempt) > 0 {
			preempt = tools.SanitizeExploreToolCalls(req.Messages, preempt, normalizedTools)
			log.Printf("preemptive tool_calls=%v", tools.ToolCallNames(preempt))
			if useStream {
				s.streamToolCalls(w, &req, preempt)
				return
			}
			s.writeToolCallCompletion(w, &req, preempt)
			return
		}
	}

	if useStream {
		s.streamResponse(w, c, &req, system, prompt, threadID, latestUser, sessionKey, settings, toolsActive, ideAgent, normalizedTools)
		return
	}

	result, err := c.Complete(prompt, system, req.Model, threadID, latestUser, ideAgent, toolsActive, normalizedTools)
	if err != nil {
		writeNotionError(w, err)
		return
	}
	result = s.bridgeIDEAgent(result, &req, normalizedTools, prompt, ideAgent, toolsActive, c, system)

	if !ideAgent && !toolsActive {
		s.rememberThread(sessionKey, result.ThreadID, req.Model, req.Messages)
	}

	finishReason := "stop"
	if len(result.ToolCalls) > 0 {
		finishReason = "tool_calls"
	}
	writeJSON(w, map[string]any{
		"id":      "chatcmpl-" + uuid.New().String()[:24],
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]any{{
			"index": 0, "message": assistantMessage(result), "finish_reason": finishReason,
		}},
		"usage": map[string]int{
			"prompt_tokens": result.InputTokens, "completion_tokens": result.OutputTokens,
			"total_tokens": result.InputTokens + result.OutputTokens,
		},
	})
}

func (s *Server) streamResponse(
	w http.ResponseWriter, c *client.NotionAIClient, req *chatRequest,
	system, prompt, threadID, latestUser, sessionKey string, settings *config.Settings,
	toolsActive, ideAgent bool, normalizedTools []map[string]any,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	completionID := "chatcmpl-" + uuid.New().String()[:24]
	created := time.Now().Unix()

	writeChunk(w, completionID, created, req.Model, map[string]any{"role": "assistant", "content": ""}, nil)
	flusher.Flush()

	done := make(chan struct{})
	defer close(done)
	go streamKeepalive(w, flusher, done)

	handle, err := c.StreamDeltas(prompt, system, req.Model, threadID, latestUser, ideAgent, toolsActive, normalizedTools)
	if err != nil {
		errJSON, _ := json.Marshal(map[string]any{
			"error": map[string]any{"message": err.Error(), "type": "notion_error", "code": errors.HTTPStatus(err)},
		})
		fmt.Fprintf(w, "data: %s\n\n", errJSON)
		flusher.Flush()
		return
	}

	var buffered []string
	for delta := range handle.Deltas {
		buffered = append(buffered, delta)
		if !toolsActive {
			writeChunk(w, completionID, created, req.Model, map[string]any{"content": delta}, nil)
			flusher.Flush()
		}
	}

	result, err := handle.Finalize()
	if err != nil {
		empty := &client.ChatResult{ThreadID: handle.ThreadID, Model: req.Model}
		result = s.bridgeIDEResult(empty, req, normalizedTools, prompt, ideAgent, toolsActive, c, system)
		if result == nil || (len(result.ToolCalls) == 0 && strings.TrimSpace(result.Text) == "") {
			errJSON, _ := json.Marshal(map[string]any{
				"error": map[string]any{"message": err.Error(), "type": "notion_error", "code": errors.HTTPStatus(err)},
			})
			fmt.Fprintf(w, "data: %s\n\n", errJSON)
			flusher.Flush()
			return
		}
	} else {
		result = s.bridgeIDEResult(result, req, normalizedTools, prompt, ideAgent, toolsActive, c, system)
	}

	if !ideAgent && !toolsActive && result != nil {
		s.rememberThread(sessionKey, result.ThreadID, req.Model, req.Messages)
	}

	if result != nil && len(result.ToolCalls) > 0 {
		emitToolCallChunks(w, flusher, completionID, created, req.Model, result.ToolCalls)
	} else if toolsActive && tools.LooksLikeToolDenial(strings.Join(buffered, "")) {
		if preempt := tools.PreemptiveAgentToolCalls(req.Messages, normalizedTools); len(preempt) > 0 {
			emitToolCallChunks(w, flusher, completionID, created, req.Model, preempt)
		} else {
			writeChunk(w, completionID, created, req.Model, map[string]any{}, strPtr("stop"))
		}
	} else {
		pieces := buffered
		if len(pieces) == 0 && result != nil && result.Text != "" {
			pieces = []string{result.Text}
		}
		for _, piece := range pieces {
			writeChunk(w, completionID, created, req.Model, map[string]any{"content": piece}, nil)
			flusher.Flush()
		}
		writeChunk(w, completionID, created, req.Model, map[string]any{}, strPtr("stop"))
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (s *Server) bridgeIDEAgent(
	result *client.ChatResult, req *chatRequest, normalizedTools []map[string]any,
	prompt string, ideAgent, toolsActive bool, c *client.NotionAIClient, system string,
) *client.ChatResult {
	return s.bridgeIDEResult(result, req, normalizedTools, prompt, ideAgent, toolsActive, c, system)
}

func (s *Server) bridgeIDEResult(
	result *client.ChatResult, req *chatRequest, normalizedTools []map[string]any,
	prompt string, ideAgent, toolsActive bool, c *client.NotionAIClient, system string,
) *client.ChatResult {
	if result == nil {
		return nil
	}
	if !ideAgent || !toolsActive {
		return result
	}
	text, toolCalls := tools.BridgeIDEAgentResponse(req.Messages, result.Text, result.ToolCalls, normalizedTools, prompt)
	if len(toolCalls) > 0 {
		result.Text = text
		result.ToolCalls = toolCalls
		return result
	}
	denial := tools.LooksLikeToolDenial(result.Text)
	if denial {
		result.Text = ""
		result.ToolCalls = nil
	} else if text != "" {
		result.Text = text
	}
	if len(result.ToolCalls) == 0 && (denial || tools.LooksLikeCodingTaskPrompt(prompt)) {
		retrySystem := strings.TrimSpace(system)
		appendText := tools.BuildToolDenialRetryAppend()
		if retrySystem != "" {
			retrySystem += "\n\n" + appendText
		} else {
			retrySystem = appendText
		}
		retry, err := c.Complete(prompt, retrySystem, req.Model, "", tools.ExtractLastUserMessage(req.Messages), ideAgent, toolsActive, normalizedTools)
		if err == nil {
			return s.bridgeIDEResult(retry, req, normalizedTools, prompt, ideAgent, toolsActive, c, system)
		}
	}
	return result
}

func (s *Server) resolvedModel(model string, settings *config.Settings) string {
	return models.ResolveModel(models.NormalizeRequestModel(model), settings.DefaultModel, models.GetCachedAliasMap())
}

func (s *Server) resolveThreadID(sessionKey, model string, messages []tools.ChatMessage) string {
	resolved := s.resolvedModel(model, config.Get())
	keys := tools.AllSessionKeys("", s.settings.APIKey, messages)
	if sessionKey != "" {
		keys = append([]string{sessionKey}, keys...)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	var ordered []string
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		ordered = append(ordered, key)
	}
	for _, key := range ordered {
		if prev, ok := s.sessionModels[key]; ok && prev != resolved {
			delete(s.sessionThreads, key)
			continue
		}
		if tid, ok := s.sessionThreads[key]; ok && tid != "" {
			for _, k := range ordered {
				s.sessionThreads[k] = tid
				s.sessionModels[k] = resolved
			}
			return tid
		}
	}
	return ""
}

func (s *Server) rememberThread(sessionKey, threadID, model string, messages []tools.ChatMessage) {
	if threadID == "" {
		return
	}
	resolved := s.resolvedModel(model, config.Get())
	keys := tools.AllSessionKeys("", s.settings.APIKey, messages)
	if sessionKey != "" {
		keys = append([]string{sessionKey}, keys...)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		s.sessionThreads[key] = threadID
		s.sessionModels[key] = resolved
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (s *Server) ensureModelAliases(c *client.NotionAIClient, settings *config.Settings) {
	if models.GetCachedAliasMap() != nil {
		return
	}
	raw, err := c.FetchAvailableModels()
	if err != nil {
		log.Printf("Could not prefetch Notion model aliases: %v", err)
		return
	}
	data := models.ListOpenAIModelsFromNotion(raw)
	models.CacheOpenAIModels(data, models.ParseAvailableModels(raw))
}

func assistantMessage(result *client.ChatResult) map[string]any {
	msg := map[string]any{"role": "assistant"}
	if len(result.ToolCalls) > 0 {
		msg["content"] = result.Text
		msg["tool_calls"] = result.ToolCalls
	} else {
		msg["content"] = result.Text
	}
	return msg
}

func (s *Server) streamToolCalls(w http.ResponseWriter, req *chatRequest, toolCalls []map[string]any) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	completionID := "chatcmpl-" + uuid.New().String()[:24]
	created := time.Now().Unix()
	writeChunk(w, completionID, created, req.Model, map[string]any{"role": "assistant", "content": ""}, nil)
	flusher.Flush()
	emitToolCallChunks(w, flusher, completionID, created, req.Model, toolCalls)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (s *Server) writeToolCallCompletion(w http.ResponseWriter, req *chatRequest, toolCalls []map[string]any) {
	result := &client.ChatResult{ToolCalls: toolCalls, Model: req.Model}
	writeJSON(w, map[string]any{
		"id":      "chatcmpl-" + uuid.New().String()[:24],
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]any{{
			"index": 0, "message": assistantMessage(result), "finish_reason": "tool_calls",
		}},
		"usage": map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	})
}

func emitToolCallChunks(
	w http.ResponseWriter, flusher http.Flusher,
	completionID string, created int64, model string, toolCalls []map[string]any,
) {
	writeChunk(w, completionID, created, model, map[string]any{"role": "assistant", "content": nil}, nil)
	flusher.Flush()
	for i, tc := range toolCalls {
		fn, _ := tc["function"].(map[string]any)
		writeChunk(w, completionID, created, model, map[string]any{
			"tool_calls": []map[string]any{{
				"index": i, "id": tc["id"], "type": "function",
				"function": map[string]any{"name": fn["name"], "arguments": ""},
			}},
		}, nil)
		flusher.Flush()
		args := fmt.Sprint(fn["arguments"])
		step := len(args) / 4
		if step < 1 {
			step = 1
		}
		for pos := 0; pos < len(args); pos += step {
			end := pos + step
			if end > len(args) {
				end = len(args)
			}
			writeChunk(w, completionID, created, model, map[string]any{
				"tool_calls": []map[string]any{{
					"index": i, "function": map[string]any{"arguments": args[pos:end]},
				}},
			}, nil)
			flusher.Flush()
		}
	}
	writeChunk(w, completionID, created, model, map[string]any{}, strPtr("tool_calls"))
	flusher.Flush()
}

func streamKeepalive(w http.ResponseWriter, flusher http.Flusher, done <-chan struct{}) {
	ticker := time.NewTicker(12 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func writeChunk(w http.ResponseWriter, id string, created int64, model string, delta map[string]any, finish *string) {
	payload := map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
	}
	b, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": msg}})
}

func writeNotionError(w http.ResponseWriter, err error) {
	code := errors.HTTPStatus(err)
	writeError(w, err.Error(), code)
}

func strPtr(s string) *string { return &s }