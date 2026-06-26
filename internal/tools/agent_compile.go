package tools

import (
	"encoding/json"
	"regexp"
	"runtime"
	"strings"

	"github.com/google/uuid"

	"github.com/mughu-id/notionchat/internal/config"
)

var (
	shellFenceRe = regexp.MustCompile("(?is)```(?:bash|sh|shell|zsh|powershell|terminal|cmd)?\\s*\\n([\\s\\S]*?)```")
	cmdLineRe    = regexp.MustCompile(`(?im)^(?:npm|npx|pnpm|yarn|bun|cd|mkdir|curl|git|ls|dir|Get-ChildItem|rg|grep|find)\b.+$`)
	winPathRe    = regexp.MustCompile(`[A-Za-z]:\\(?:[^\\\n]+\\)*[^\\\n]+`)
	unixPathRe   = regexp.MustCompile(`/(?:[\w.-]+/)*[\w.-]+`)
	filePathRe   = regexp.MustCompile(`(?i)(?:@)?(?:[\w.-]+[\\/])+[\w.-]+\.(?:go|ts|tsx|js|jsx|py|md|json|rs|java|cs|yaml|yml|txt|html|css|vue|toml|mod|sum|xml|gradle)`)
)

func pickTool(clientTools []map[string]any, candidates ...string) string {
	allowed := clientToolNames(clientTools)
	for _, name := range candidates {
		if allowed[name] {
			return name
		}
	}
	lower := make(map[string]string)
	for n := range allowed {
		lower[strings.ToLower(n)] = n
	}
	for _, c := range candidates {
		if n, ok := lower[strings.ToLower(c)]; ok {
			return n
		}
	}
	return pickToolByHint(clientTools, candidates...)
}

func isMCPTool(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "mcp") || strings.HasSuffix(lower, "_resource") || strings.HasSuffix(lower, "_resources")
}

func pickToolByHint(clientTools []map[string]any, hints ...string) string {
	allowed := clientToolNames(clientTools)
	for name := range allowed {
		if isMCPTool(name) {
			continue
		}
		lower := strings.ToLower(name)
		for _, hint := range hints {
			if strings.Contains(lower, strings.ToLower(hint)) {
				return name
			}
		}
	}
	return ""
}

func pickShellTool(clientTools []map[string]any) string {
	if t := pickTool(clientTools, "shell_command", "Shell", "run_terminal_cmd", "run_terminal_command", "shell", "exec", "local_shell"); t != "" && !isMCPTool(t) {
		return t
	}
	return pickToolByHint(clientTools, "shell", "terminal", "command", "exec", "bash")
}

func pickGlobTool(clientTools []map[string]any) string {
	if t := pickTool(clientTools, "glob_file_search", "Glob", "glob", "list_dir", "list_files"); t != "" && !isMCPTool(t) {
		return t
	}
	return pickToolByHint(clientTools, "glob", "list_dir", "find_file")
}

func pickReadTool(clientTools []map[string]any) string {
	if t := pickTool(clientTools, "read_file", "Read", "read"); t != "" && !isMCPTool(t) {
		return t
	}
	return pickToolByHint(clientTools, "read_file", "read")
}

func pickExploreTool(clientTools []map[string]any) string {
	if t := pickShellTool(clientTools); t != "" {
		return t
	}
	if t := pickGlobTool(clientTools); t != "" {
		return t
	}
	return ""
}

func isWindowsContext(messages []ChatMessage, path string) bool {
	if strings.EqualFold(config.Get().ClientOS, "windows") {
		return true
	}
	if path != "" && (strings.Contains(path, "\\") || strings.Contains(path, ":")) {
		return true
	}
	for _, msg := range messages {
		text := extractText(msg.Content)
		if winPathRe.MatchString(text) || strings.Contains(text, "\\") {
			return true
		}
		lower := strings.ToLower(text)
		if strings.Contains(lower, "windows") || strings.Contains(lower, "powershell") {
			return true
		}
	}
	return runtime.GOOS == "windows"
}

func windowsListCommand(path string) string {
	if path != "" {
		return `Get-ChildItem -Force "` + path + `"`
	}
	return "Get-ChildItem -Force"
}

func unixListCommand(path string) string {
	if path != "" {
		return `ls -la "` + path + `"`
	}
	return "ls -la"
}

func buildExploreArgs(toolName, path string, messages []ChatMessage) map[string]any {
	lower := strings.ToLower(toolName)
	windows := isWindowsContext(messages, path)
	switch {
	case strings.Contains(lower, "shell") || strings.Contains(lower, "command") || lower == "exec":
		cmd := unixListCommand(path)
		if windows {
			cmd = windowsListCommand(path)
		}
		return map[string]any{"command": cmd, "description": "List project files for codebase analysis"}
	case strings.Contains(lower, "list_dir"):
		args := map[string]any{}
		if path != "" {
			args["path"] = path
		}
		return args
	default:
		args := map[string]any{"pattern": "**/*"}
		if path != "" {
			args["target_directory"] = path
		}
		return args
	}
}



func makeToolCall(name string, args map[string]any) map[string]any {
	b, _ := json.Marshal(args)
	return map[string]any{
		"id":   "call_" + uuid.New().String()[:24],
		"type": "function",
		"function": map[string]any{
			"name":      name,
			"arguments": string(b),
		},
	}
}

func dedupeToolCalls(toolCalls []map[string]any) []map[string]any {
	seen := map[string]bool{}
	var out []map[string]any
	for _, tc := range NormalizeToolCalls(toolCalls) {
		fn, _ := tc["function"].(map[string]any)
		key := stringVal(fn["name"]) + "::" + stringVal(fn["arguments"])
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, tc)
	}
	return out
}

func extractAllToolCallsFromText(text string) []map[string]any {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if obj := tryParseJSONObject(text); obj != nil {
		if raw := obj["tool_calls"]; raw != nil {
			return NormalizeToolCalls(raw)
		}
	}
	var found []map[string]any
	needle := `"tool_calls"`
	for idx := strings.Index(text, needle); idx >= 0; {
		brace := strings.LastIndex(text[:idx], "{")
		if brace < 0 {
			break
		}
		depth := 0
		parsed := false
		for i := brace; i < len(text); i++ {
			switch text[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					var chunk map[string]any
					if err := json.Unmarshal([]byte(text[brace:i+1]), &chunk); err == nil {
						if raw := chunk["tool_calls"]; raw != nil {
							found = append(found, NormalizeToolCalls(raw)...)
						}
					}
					parsed = true
					break
				}
			}
		}
		if !parsed {
			break
		}
		rest := text[idx+len(needle):]
		if next := strings.Index(rest, needle); next >= 0 {
			text = rest[next:]
			idx = 0
		} else {
			break
		}
	}
	return dedupeToolCalls(found)
}

func extractShellCommands(text string) []string {
	var commands []string
	for _, m := range shellFenceRe.FindAllStringSubmatch(text, -1) {
		if len(m) < 2 {
			continue
		}
		for _, line := range strings.Split(m[1], "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if cmdLineRe.MatchString(line) {
				commands = append(commands, line)
			}
		}
	}
	for _, m := range cmdLineRe.FindAllString(text, -1) {
		commands = append(commands, strings.TrimSpace(m))
	}
	seen := map[string]bool{}
	var out []string
	for _, cmd := range commands {
		if !seen[cmd] {
			seen[cmd] = true
			out = append(out, cmd)
		}
	}
	return out
}

func synthesizeShellToolCalls(notionText string, clientTools []map[string]any) []map[string]any {
	shellTool := pickTool(clientTools, "Shell", "run_terminal_cmd", "run_terminal_command", "shell", "exec")
	if shellTool == "" {
		return nil
	}
	commands := extractShellCommands(notionText)
	if len(commands) == 0 {
		return nil
	}
	limit := 3
	if len(commands) < limit {
		limit = len(commands)
	}
	var out []map[string]any
	for _, cmd := range commands[:limit] {
		out = append(out, makeToolCall(shellTool, map[string]any{
			"command":     cmd,
			"description": truncateForTool(cmd, 120),
		}))
	}
	return out
}

func extractFilePathFromRequest(request string) string {
	if m := filePathRe.FindString(strings.TrimSpace(request)); m != "" {
		return strings.TrimPrefix(strings.Trim(m, `"'`), "@")
	}
	return ""
}

func extractPathFromRequest(request string) string {
	if file := extractFilePathFromRequest(request); file != "" {
		return file
	}
	if m := winPathRe.FindString(request); m != "" {
		return m
	}
	if m := unixPathRe.FindString(request); m != "" {
		return m
	}
	return ""
}

func isScaffoldShellCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	return strings.Contains(lower, "npm create") ||
		strings.Contains(lower, "create-vite") ||
		strings.Contains(lower, "create vite") ||
		strings.Contains(lower, "create next") ||
		strings.Contains(lower, "pnpm create") ||
		strings.Contains(lower, "yarn create")
}

func fileReadShellCommand(path string, messages []ChatMessage) string {
	if isWindowsContext(messages, path) {
		return `Get-Content -Raw "` + path + `"`
	}
	return `cat "` + path + `"`
}

func buildReadToolCall(path string, clientTools []map[string]any, messages []ChatMessage) []map[string]any {
	readTool := pickReadTool(clientTools)
	if readTool != "" {
		args := map[string]any{"path": path}
		if strings.Contains(strings.ToLower(readTool), "mcp") {
			args = map[string]any{"uri": path, "path": path}
		}
		return []map[string]any{makeToolCall(readTool, args)}
	}
	if shell := pickShellTool(clientTools); shell != "" {
		return []map[string]any{makeToolCall(shell, map[string]any{
			"command":     fileReadShellCommand(path, messages),
			"description": "Read file for analysis: " + path,
		})}
	}
	return nil
}

func shouldPreemptiveRead(messages []ChatMessage) (string, bool) {
	request := ExtractLastUserMessage(messages)
	file := extractFilePathFromRequest(request)
	if file == "" || !looksLikeExploreTask(request) {
		return "", false
	}
	if conversationHasFileReadResult(messages, file) {
		return "", false
	}
	return file, true
}

func conversationHasFileReadResult(messages []ChatMessage, path string) bool {
	needle := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	base := needle
	if idx := strings.LastIndexAny(needle, `/\`); idx >= 0 {
		base = needle[idx+1:]
	}
	for _, msg := range messages {
		if msg.Role != "tool" {
			continue
		}
		text := extractText(msg.Content)
		if len(text) < 80 {
			continue
		}
		lower := strings.ToLower(text)
		mentionsPath := strings.Contains(lower, needle) || (base != "" && strings.Contains(lower, base))
		if !mentionsPath {
			continue
		}
		if strings.Contains(text, "{") || strings.Contains(text, "func ") ||
			strings.Contains(text, "package ") || strings.Contains(text, "import ") ||
			strings.Contains(text, "class ") || strings.Count(text, "\n") >= 5 {
			return true
		}
	}
	return false
}

func looksLikeExploreTask(text string) bool {
	lower := strings.ToLower(text)
	hints := []string{
		"analisa", "analisis", "analyze", "analysis", "audit", "explore", "scan",
		"codebase", "repository", "repo", "project structure", "struktur",
		"read file", "list file", "baca file", "lihat file",
	}
	for _, h := range hints {
		if strings.Contains(lower, h) {
			return true
		}
	}
	return false
}

func conversationHasDirectoryListing(messages []ChatMessage) bool {
	for _, msg := range messages {
		if msg.Role != "tool" {
			continue
		}
		text := extractText(msg.Content)
		if len(text) < 80 {
			continue
		}
		if strings.Count(text, "\n") >= 5 {
			return true
		}
		lower := strings.ToLower(text)
		if strings.Contains(lower, "get-childitem") ||
			strings.Contains(lower, "node_modules") ||
			strings.Contains(lower, "directory of") ||
			strings.Contains(lower, "<dir>") {
			return true
		}
	}
	return false
}

func isExploreListCommand(args map[string]any) bool {
	cmd := strings.ToLower(strings.TrimSpace(stringVal(args["command"])))
	if cmd == "" {
		return false
	}
	return strings.Contains(cmd, "get-childitem") ||
		strings.HasPrefix(cmd, "ls") ||
		strings.Contains(cmd, "dir ") ||
		strings.Contains(cmd, "list_dir")
}

func isListExploreToolCall(tc map[string]any) bool {
	fn, _ := tc["function"].(map[string]any)
	name := strings.ToLower(stringVal(fn["name"]))
	if isMCPTool(name) || strings.Contains(name, "glob") || strings.Contains(name, "list_dir") {
		return true
	}
	var args map[string]any
	_ = json.Unmarshal([]byte(stringVal(fn["arguments"])), &args)
	return isExploreListCommand(args)
}

// ShouldExploreBootstrap reports whether the client still needs an initial directory listing.
func ShouldExploreBootstrap(messages []ChatMessage) bool {
	if conversationHasDirectoryListing(messages) || conversationHasUsefulToolResults(messages) {
		return false
	}
	request := ExtractLastUserMessage(messages)
	if request == "" || !looksLikeExploreTask(request) {
		return false
	}
	if extractFilePathFromRequest(request) != "" {
		return false
	}
	return true
}

func buildExploreToolCalls(messages []ChatMessage, clientTools []map[string]any) []map[string]any {
	if !ShouldExploreBootstrap(messages) {
		return nil
	}
	request := ExtractLastUserMessage(messages)
	if request == "" {
		return nil
	}

	path := extractPathFromRequest(request)
	if tool := pickExploreTool(clientTools); tool != "" {
		calls := []map[string]any{makeToolCall(tool, buildExploreArgs(tool, path, messages))}
		return SanitizeExploreToolCalls(messages, calls, clientTools)
	}
	return nil
}

func bootstrapExploreToolCalls(messages []ChatMessage, notionText string, clientTools []map[string]any) []map[string]any {
	if !LooksLikeToolDenial(notionText) && strings.TrimSpace(notionText) != "" {
		return nil
	}
	return buildExploreToolCalls(messages, clientTools)
}

func isEmptyMCPToolResult(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	return strings.Contains(lower, `"resources":[]`) || strings.Contains(lower, `"resources": []`)
}

func isFailedShellResult(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "namedparameternotfound") ||
		strings.Contains(lower, "fullyqualifiederrorid") ||
		strings.Contains(lower, "is not recognized") ||
		(strings.Contains(lower, "parameter") && strings.Contains(lower, "cannot be found"))
}

func fixShellCommandArgs(args map[string]any, messages []ChatMessage, path string) map[string]any {
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return args
	}
	if !isWindowsContext(messages, path) {
		return args
	}
	lower := strings.ToLower(strings.TrimSpace(cmd))
	if strings.HasPrefix(lower, "ls") || strings.Contains(lower, "ls -") {
		out := map[string]any{}
		for k, v := range args {
			out[k] = v
		}
		out["command"] = windowsListCommand(path)
		return out
	}
	return args
}

// SanitizeExploreToolCalls rewrites MCP/wrong explore tool_calls into shell_command with OS-correct listing.
func SanitizeExploreToolCalls(messages []ChatMessage, toolCalls []map[string]any, clientTools []map[string]any) []map[string]any {
	if len(toolCalls) == 0 {
		return toolCalls
	}
	if conversationHasDirectoryListing(messages) {
		var kept []map[string]any
		for _, tc := range NormalizeToolCalls(toolCalls) {
			if isListExploreToolCall(tc) {
				continue
			}
			kept = append(kept, tc)
		}
		return kept
	}
	path := extractPathFromRequest(ExtractLastUserMessage(messages))
	shellTool := pickShellTool(clientTools)
	var out []map[string]any
	for _, tc := range NormalizeToolCalls(toolCalls) {
		fn, _ := tc["function"].(map[string]any)
		name := stringVal(fn["name"])
		argsRaw := stringVal(fn["arguments"])
		var args map[string]any
		_ = json.Unmarshal([]byte(argsRaw), &args)
		if args == nil {
			args = map[string]any{}
		}

		if isMCPTool(name) || (name != "" && strings.Contains(strings.ToLower(argsRaw), "glob_pattern")) {
			if shellTool != "" {
				out = append(out, makeToolCall(shellTool, buildExploreArgs(shellTool, path, messages)))
				continue
			}
		}

		if strings.Contains(strings.ToLower(name), "shell") || strings.Contains(strings.ToLower(name), "command") {
			fixed := fixShellCommandArgs(args, messages, path)
			if cmd := stringVal(fixed["command"]); isScaffoldShellCommand(cmd) && looksLikeExploreTask(ExtractLastUserMessage(messages)) {
				if read := buildReadToolCall(extractFilePathFromRequest(ExtractLastUserMessage(messages)), clientTools, messages); len(read) > 0 {
					out = append(out, read...)
				}
				continue
			}
			out = append(out, makeToolCall(name, fixed))
			continue
		}
		out = append(out, tc)
	}
	return dedupeToolCalls(out)
}

func conversationHasUsefulToolResults(messages []ChatMessage) bool {
	for _, msg := range messages {
		if msg.Role != "tool" {
			continue
		}
		text := strings.TrimSpace(extractText(msg.Content))
		if text == "" || isEmptyMCPToolResult(text) || isFailedShellResult(text) {
			continue
		}
		return true
	}
	return false
}

// NeedsAgentTooling reports whether this request still needs a local tool round-trip.
func NeedsAgentTooling(messages []ChatMessage) bool {
	if _, ok := shouldPreemptiveRead(messages); ok {
		return true
	}
	return ShouldExploreBootstrap(messages)
}

// AgentFallbackToolCalls always returns a concrete tool for explore/read tasks when possible.
func AgentFallbackToolCalls(messages []ChatMessage, clientTools []map[string]any) []map[string]any {
	if calls := PreemptiveAgentToolCalls(messages, clientTools); len(calls) > 0 {
		return SanitizeExploreToolCalls(messages, calls, clientTools)
	}
	return nil
}

// PreemptiveAgentToolCalls returns tool_calls before calling Notion when the client
// still needs filesystem exploration (Codex first turn on analyze/list tasks).
func PreemptiveAgentToolCalls(messages []ChatMessage, clientTools []map[string]any) []map[string]any {
	if file, ok := shouldPreemptiveRead(messages); ok {
		if calls := buildReadToolCall(file, clientTools, messages); len(calls) > 0 {
			return calls
		}
	}
	return buildExploreToolCalls(messages, clientTools)
}

func ExploreToolCallsIssued(toolCalls []map[string]any) bool {
	for _, tc := range NormalizeToolCalls(toolCalls) {
		if isListExploreToolCall(tc) {
			return true
		}
	}
	return false
}

func bootstrapScaffoldToolCalls(messages []ChatMessage, notionText string, clientTools []map[string]any) []map[string]any {
	request := ExtractLastUserMessage(messages)
	if looksLikeExploreTask(request) || !LooksLikeScaffoldTask(request) {
		return nil
	}
	if !LooksLikeToolDenial(notionText) && strings.TrimSpace(notionText) != "" {
		return nil
	}
	shellTool := pickShellTool(clientTools)
	if shellTool == "" {
		return nil
	}
	return []map[string]any{makeToolCall(shellTool, map[string]any{
		"command":     "npm create vite@latest . -- --template react-ts",
		"description": "Scaffold project in current workspace",
	})}
}

func CompileAgentToolCalls(
	messages []ChatMessage,
	notionText string,
	notionToolCalls []map[string]any,
	clientTools []map[string]any,
	prompt string,
) (string, []map[string]any) {
	text := notionText
	var collected []map[string]any

	collected = append(collected, extractAllToolCallsFromText(text)...)
	content, parsed := ParseAssistantOutput(text)
	if len(parsed) > 0 {
		collected = append(collected, parsed...)
	}
	if len(notionToolCalls) > 0 {
		collected = append(collected, AlignToolCallsToClient(notionToolCalls, clientTools, true)...)
	}

	collected = dedupeToolCalls(collected)
	if len(collected) > 0 {
		return content, SanitizeExploreToolCalls(messages, collected, clientTools)
	}

	if shells := synthesizeShellToolCalls(text, clientTools); len(shells) > 0 {
		return "", SanitizeExploreToolCalls(messages, shells, clientTools)
	}
	if explore := bootstrapExploreToolCalls(messages, text, clientTools); len(explore) > 0 {
		return "", explore
	}
	if scaffold := bootstrapScaffoldToolCalls(messages, text, clientTools); len(scaffold) > 0 {
		return "", scaffold
	}
	if LooksLikeToolDenial(text) {
		return "", nil
	}
	return notionText, nil
}

func ToolCallNames(toolCalls []map[string]any) []string {
	var names []string
	for _, tc := range NormalizeToolCalls(toolCalls) {
		fn, _ := tc["function"].(map[string]any)
		if name := stringVal(fn["name"]); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func truncateForTool(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}