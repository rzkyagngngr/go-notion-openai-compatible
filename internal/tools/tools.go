package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/mughu-id/notionchat/internal/errors"
)

var jsonBlockRe = regexp.MustCompile("(?is)```(?:json)?\\s*(\\{.*?\\})\\s*```")

var denialPhrases = []string{
	"i'm notion ai", "i am notion ai", "can't directly create", "cannot directly create",
	"don't have access to your local", "cannot access your workspace",
	"can't access your workspace", "don't have access to cursor",
}

type ChatMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"`
	ToolCalls  []map[string]any `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

func NormalizeTools(tools []map[string]any) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	var out []map[string]any
	for _, tool := range tools {
		if tool["type"] == "function" {
			if fn, ok := tool["function"].(map[string]any); ok {
				out = append(out, tool)
				_ = fn
				continue
			}
		}
		if name, ok := tool["name"].(string); ok {
			out = append(out, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        name,
					"description": tool["description"],
					"parameters":  tool["parameters"],
				},
			})
		}
	}
	return out
}

func IsIDEAgentMessages(messages []ChatMessage) bool {
	for _, msg := range messages {
		if msg.Role != "system" {
			continue
		}
		text := strings.ToLower(extractText(msg.Content))
		if text == "" {
			continue
		}
		agentMarkers := []string{"cursor", "composer", "coding assistant", "tool_calls", "function calling"}
		toolMarkers := []string{"read", "glob", "strreplace", "run_terminal", "codebase_search"}
		hasAgent, hasTool := false, false
		for _, m := range agentMarkers {
			if strings.Contains(text, m) {
				hasAgent = true
			}
		}
		for _, m := range toolMarkers {
			if strings.Contains(text, m) {
				hasTool = true
			}
		}
		if hasAgent && hasTool {
			return true
		}
	}
	return false
}

func IsIDEAgentTools(tools []map[string]any) bool {
	ideExact := map[string]bool{
		"read": true, "write": true, "shell": true, "glob": true, "grep": true,
		"read_file": true, "write_file": true, "run_terminal_cmd": true,
	}
	for _, tool := range NormalizeTools(tools) {
		fn, _ := tool["function"].(map[string]any)
		name := strings.ToLower(stringVal(fn["name"]))
		if ideExact[name] {
			return true
		}
		for _, part := range []string{"file", "terminal", "grep", "write", "shell"} {
			if strings.Contains(name, part) {
				return true
			}
		}
	}
	return false
}

func CursorFallbackTools() []map[string]any {
	names := []string{"Glob", "Read", "Write", "StrReplace", "Shell", "Grep", "SemanticSearch", "Delete", "ReadLints"}
	var out []map[string]any
	for _, name := range names {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":       name,
				"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
			},
		})
	}
	return out
}

func ExtractLastUserMessage(messages []ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		if text := strings.TrimSpace(extractUserText(messages[i].Content)); text != "" {
			return text
		}
	}
	return ""
}

func FirstUserMessage(messages []ChatMessage) string {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		if text := strings.TrimSpace(extractUserText(msg.Content)); text != "" {
			return text
		}
	}
	return ""
}

func AllSessionKeys(user, apiKey string, messages []ChatMessage) []string {
	seen := map[string]bool{}
	var keys []string
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	add(strings.TrimSpace(user))
	add(SessionKeyFromMessages(user, apiKey, messages))
	if first := FirstUserMessage(messages); first != "" {
		sum := sha256.Sum256([]byte(first))
		add("auto:" + hex.EncodeToString(sum[:8]))
	}
	if apiKey != "" {
		sum := sha256.Sum256([]byte(apiKey))
		add("sticky:" + hex.EncodeToString(sum[:8]))
	}
	return keys
}

func SessionKeyFromMessages(user, apiKey string, messages []ChatMessage) string {
	if strings.TrimSpace(user) != "" {
		return strings.TrimSpace(user)
	}

	hasAssistant := false
	userCount := 0
	firstUser := ""
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			userCount++
			if firstUser == "" {
				if text := strings.TrimSpace(extractUserText(msg.Content)); text != "" {
					firstUser = text
				}
			}
		case "assistant":
			hasAssistant = true
		}
	}

	// Client mengirim full history (user + assistant + user) → anchor ke pesan user pertama.
	if hasAssistant || userCount > 1 {
		if firstUser != "" {
			sum := sha256.Sum256([]byte(firstUser))
			return "auto:" + hex.EncodeToString(sum[:8])
		}
	}

	// Client hanya kirim pesan terakhir (Cursor/UI tanpa history) → sticky per API key.
	if apiKey != "" {
		sum := sha256.Sum256([]byte(apiKey))
		return "sticky:" + hex.EncodeToString(sum[:8])
	}
	if firstUser != "" {
		sum := sha256.Sum256([]byte(firstUser))
		return "auto:" + hex.EncodeToString(sum[:8])
	}
	return ""
}

func PrepareChatInput(messages []ChatMessage, tools []map[string]any, toolChoice any) (system, prompt string, toolsActive, ideAgent bool, normalized []map[string]any, err error) {
	cursorIDE := IsIDEAgentMessages(messages)
	normalized = NormalizeTools(tools)
	if len(normalized) == 0 && cursorIDE {
		normalized = CursorFallbackTools()
	}
	toolsActive = len(normalized) > 0 && toolChoice != "none"
	ideAgent = toolsActive && (IsIDEAgentTools(normalized) || cursorIDE)

	systemParts := []string{
		"You are a helpful assistant. Respond directly in the conversation. " +
			"Do not create, draft, or render Notion pages. Answer inline in the conversation.",
	}
	var transcriptBlocks []string
	var pendingToolResults []string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if text := strings.TrimSpace(extractText(msg.Content)); text != "" {
				systemParts = append(systemParts, text)
			}
		case "user":
			if len(pendingToolResults) > 0 {
				transcriptBlocks = append(transcriptBlocks, pendingToolResults...)
				pendingToolResults = nil
			}
			text := extractUserText(msg.Content)
			if text != "" {
				transcriptBlocks = append(transcriptBlocks, "User: "+text)
			}
		case "assistant":
			for _, tc := range msg.ToolCalls {
				fn, _ := tc["function"].(map[string]any)
				name := stringVal(fn["name"])
				args := stringVal(fn["arguments"])
				transcriptBlocks = append(transcriptBlocks, "Assistant: [tool call `"+name+"` args="+args+"]")
			}
			if text := strings.TrimSpace(extractText(msg.Content)); text != "" {
				transcriptBlocks = append(transcriptBlocks, "Assistant: "+text)
			}
		case "tool":
			label := msg.Name
			if label == "" {
				label = msg.ToolCallID
			}
			if label == "" {
				label = "tool"
			}
			result := strings.TrimSpace(extractText(msg.Content))
			pendingToolResults = append(pendingToolResults, "Tool `"+label+"` result:\n"+result)
		}
	}
	if len(pendingToolResults) > 0 {
		transcriptBlocks = append(transcriptBlocks, pendingToolResults...)
		transcriptBlocks = append(transcriptBlocks, "User: Continue using the tool results above.")
	}
	if len(transcriptBlocks) == 0 {
		return "", "", false, false, normalized, errors.New("No user message in request", 400)
	}
	if toolsActive {
		systemParts = append(systemParts, buildToolsSystemAppend(normalized, toolChoice, ideAgent))
	}
	system = strings.Join(systemParts, "\n\n")
	prompt = strings.Join(transcriptBlocks, "\n\n")
	return system, prompt, toolsActive, ideAgent, normalized, nil
}

func MergeToolCalls(text string, ndjsonToolCalls []map[string]any, toolsActive bool, clientTools []map[string]any, prompt string, ideAgent bool) (string, []map[string]any) {
	if !toolsActive {
		return text, nil
	}
	content, parsed := ParseAssistantOutput(text)
	if len(parsed) > 0 {
		aligned := AlignToolCallsToClient(parsed, clientTools, true)
		if len(aligned) > 0 {
			return content, aligned
		}
	}
	if len(ndjsonToolCalls) > 0 {
		aligned := AlignToolCallsToClient(ndjsonToolCalls, clientTools, ideAgent)
		if len(aligned) > 0 {
			if content != "" {
				return content, aligned
			}
			return "", aligned
		}
	}
	if content != "" {
		return content, nil
	}
	if text != "" {
		return text, nil
	}
	return "", nil
}

func BridgeIDEAgentResponse(messages []ChatMessage, notionText string, notionToolCalls []map[string]any, clientTools []map[string]any, prompt string) (string, []map[string]any) {
	content, toolCalls := ParseAssistantOutput(notionText)
	if len(toolCalls) > 0 {
		aligned := AlignToolCallsToClient(toolCalls, clientTools, true)
		if len(aligned) > 0 {
			return content, aligned
		}
	}
	if len(notionToolCalls) > 0 {
		aligned := AlignToolCallsToClient(notionToolCalls, clientTools, true)
		if len(aligned) > 0 {
			return content, aligned
		}
	}
	if LooksLikeToolDenial(notionText) {
		return "", nil
	}
	return notionText, nil
}

func LooksLikeToolDenial(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	for _, phrase := range denialPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func LooksLikeCodingTaskPrompt(prompt string) bool {
	lower := strings.ToLower(prompt)
	tail := lower
	if idx := strings.LastIndex(lower, "user:"); idx >= 0 {
		tail = lower[idx+5:]
	}
	hints := []string{"create", "build", "scaffold", "implement", "vite", "react", "app", "project"}
	for _, h := range hints {
		if strings.Contains(tail, h) {
			return true
		}
	}
	return false
}

func BuildToolDenialRetryAppend() string {
	return "Use `npm create vite@latest .` in the current folder. Call Shell scaffold alone first; after it finishes, call Write for files."
}

func ParseAssistantOutput(text string) (string, []map[string]any) {
	stripped := strings.TrimSpace(text)
	if stripped == "" {
		return "", nil
	}
	obj := tryParseJSONObject(stripped)
	if obj != nil {
		toolCalls := NormalizeToolCalls(obj["tool_calls"])
		if len(toolCalls) > 0 {
			content := stringVal(obj["content"])
			return content, toolCalls
		}
		if content := stringVal(obj["content"]); content != "" {
			return content, nil
		}
	}
	return stripped, nil
}

func NormalizeToolCalls(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		if maps, ok := raw.([]map[string]any); ok {
			items = make([]any, len(maps))
			for i, m := range maps {
				items[i] = m
			}
		} else {
			return nil
		}
	}
	var out []map[string]any
	for _, item := range items {
		tc, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := tc["function"].(map[string]any)
		name := stringVal(fn["name"])
		if name == "" {
			name = stringVal(tc["name"])
		}
		if name == "" {
			continue
		}
		args := fn["arguments"]
		if args == nil {
			args = tc["arguments"]
		}
		argsStr := "{}"
		switch v := args.(type) {
		case string:
			argsStr = v
		case map[string]any:
			b, _ := json.Marshal(v)
			argsStr = string(b)
		}
		id := stringVal(tc["id"])
		if id == "" {
			id = "call_" + uuid.New().String()[:24]
		}
		out = append(out, map[string]any{
			"id": id, "type": "function",
			"function": map[string]any{"name": name, "arguments": argsStr},
		})
	}
	return out
}

func AlignToolCallsToClient(toolCalls []map[string]any, clientTools []map[string]any, allowAliases bool) []map[string]any {
	allowed := clientToolNames(clientTools)
	if len(allowed) == 0 {
		return NormalizeToolCalls(toolCalls)
	}
	lowerToCanonical := make(map[string]string)
	for name := range allowed {
		lowerToCanonical[strings.ToLower(name)] = name
	}
	aliases := map[string]string{
		"read_file": "Read", "read": "Read", "write_file": "Write", "write": "Write",
		"run_terminal_cmd": "Shell", "list_dir": "Glob", "glob_file_search": "Glob",
	}
	if allowAliases {
		for alias, target := range aliases {
			if allowed[target] {
				lowerToCanonical[alias] = target
			}
		}
	}
	var out []map[string]any
	for _, tc := range NormalizeToolCalls(toolCalls) {
		fn, _ := tc["function"].(map[string]any)
		name := stringVal(fn["name"])
		if allowed[name] {
			out = append(out, tc)
			continue
		}
		if allowAliases {
			if mapped, ok := lowerToCanonical[strings.ToLower(name)]; ok {
				fixed := copyMap(tc)
				fnCopy := copyMap(fn)
				fnCopy["name"] = mapped
				fixed["function"] = fnCopy
				out = append(out, fixed)
			}
		}
	}
	return out
}

func clientToolNames(tools []map[string]any) map[string]bool {
	names := make(map[string]bool)
	for _, tool := range NormalizeTools(tools) {
		fn, _ := tool["function"].(map[string]any)
		if name := stringVal(fn["name"]); name != "" {
			names[name] = true
		}
	}
	return names
}

func buildToolsSystemAppend(tools []map[string]any, toolChoice any, ideAgent bool) string {
	if len(tools) == 0 || toolChoice == "none" {
		return ""
	}
	if ideAgent {
		specs, _ := json.MarshalIndent(tools, "", "  ")
		s := string(specs)
		if len(s) > 14000 {
			s = s[:14000] + "\n... (truncated)"
		}
		return "OpenAI function-calling channel for Cursor IDE.\n" +
			"Scaffold in the CURRENT workspace folder. Never say you lack filesystem access.\n\n" +
			"Tool schemas (JSON):\n" + s + "\n\n" +
			`When calling tools, respond with ONLY JSON: {"content": null, "tool_calls": [...]}`
	}
	specs, _ := json.MarshalIndent(tools, "", "  ")
	return "You are an assistant that can call external tools using OpenAI function calling.\n\n" +
		"Available tools (JSON Schema):\n" + string(specs) + "\n\n" +
		`When you need tools, respond with ONLY valid JSON: {"content": null, "tool_calls": [...]}`
}

func tryParseJSONObject(text string) map[string]any {
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err == nil {
		return obj
	}
	if match := jsonBlockRe.FindStringSubmatch(text); len(match) > 1 {
		if err := json.Unmarshal([]byte(match[1]), &obj); err == nil {
			return obj
		}
	}
	start := strings.Index(text, "{")
	if start < 0 {
		return nil
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				if err := json.Unmarshal([]byte(text[start:i+1]), &obj); err == nil {
					return obj
				}
				return nil
			}
		}
	}
	return nil
}

func extractText(content any) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	items, ok := content.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] == "text" {
			parts = append(parts, stringVal(m["text"]))
		}
	}
	return strings.Join(parts, "\n")
}

func extractUserText(content any) string {
	if s, ok := content.(string); ok {
		return strings.TrimSpace(s)
	}
	items, ok := content.([]any)
	if !ok {
		return strings.TrimSpace(extractText(content))
	}
	var parts []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] == "text" {
			parts = append(parts, stringVal(m["text"]))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func stringVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}