package tools

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	shellFenceRe = regexp.MustCompile("(?is)```(?:bash|sh|shell|zsh|powershell|terminal|cmd)?\\s*\\n([\\s\\S]*?)```")
	cmdLineRe    = regexp.MustCompile(`(?im)^(?:npm|npx|pnpm|yarn|bun|cd|mkdir|curl|git|ls|dir|Get-ChildItem|rg|grep|find)\b.+$`)
	winPathRe    = regexp.MustCompile(`[A-Za-z]:\\(?:[^\\\n]+\\)*[^\\\n]+`)
	unixPathRe   = regexp.MustCompile(`/(?:[\w.-]+/)*[\w.-]+`)
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
	return ""
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

func extractPathFromRequest(request string) string {
	if m := winPathRe.FindString(request); m != "" {
		return m
	}
	if m := unixPathRe.FindString(request); m != "" {
		return m
	}
	return ""
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

func bootstrapExploreToolCalls(messages []ChatMessage, notionText string, clientTools []map[string]any) []map[string]any {
	if !LooksLikeToolDenial(notionText) && strings.TrimSpace(notionText) != "" {
		return nil
	}
	request := ExtractLastUserMessage(messages)
	if request == "" {
		return nil
	}
	if !looksLikeExploreTask(request) && !looksLikeExploreTask(notionText) {
		return nil
	}

	globTool := pickTool(clientTools, "Glob", "glob", "glob_file_search", "list_dir", "list_files")
	shellTool := pickTool(clientTools, "Shell", "run_terminal_cmd", "run_terminal_command", "shell", "exec")
	readTool := pickTool(clientTools, "Read", "read", "read_file")

	path := extractPathFromRequest(request)
	if globTool != "" {
		pattern := "**/*"
		args := map[string]any{"glob_pattern": pattern}
		if path != "" {
			args["target_directory"] = path
		}
		return []map[string]any{makeToolCall(globTool, args)}
	}
	if shellTool != "" {
		cmd := "ls -la"
		if path != "" {
			if strings.Contains(path, ":") || strings.Contains(path, "\\") {
				cmd = `Get-ChildItem -Force "` + path + `"`
			} else {
				cmd = `ls -la "` + path + `"`
			}
		}
		return []map[string]any{makeToolCall(shellTool, map[string]any{
			"command":     cmd,
			"description": "List project files for codebase analysis",
		})}
	}
	if readTool != "" && path != "" {
		return []map[string]any{makeToolCall(readTool, map[string]any{"path": path})}
	}
	return nil
}

func bootstrapScaffoldToolCalls(messages []ChatMessage, notionText string, clientTools []map[string]any) []map[string]any {
	if !LooksLikeCodingTaskPrompt(strings.Join([]string{ExtractLastUserMessage(messages)}, "\n")) {
		return nil
	}
	if !LooksLikeToolDenial(notionText) && strings.TrimSpace(notionText) != "" {
		return nil
	}
	shellTool := pickTool(clientTools, "Shell", "run_terminal_cmd", "run_terminal_command", "shell")
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
		return content, collected
	}

	if shells := synthesizeShellToolCalls(text, clientTools); len(shells) > 0 {
		return "", shells
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

func truncateForTool(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}