package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompileAgentToolCallsFromJSON(t *testing.T) {
	text := `{"content": null, "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "Glob", "arguments": "{\"glob_pattern\":\"**/*\"}"}}]}`
	tools := CursorFallbackTools()
	_, calls := CompileAgentToolCalls(nil, text, nil, tools, "")
	if len(calls) != 1 {
		t.Fatalf("calls: %d", len(calls))
	}
}

func TestCompileAgentBootstrapExploreOnDenial(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "boleh analisa codebase di C:\\Users\\test\\poly-scan"}}
	denial := "Maaf, saya Notion AI dan tidak punya akses ke shell atau filesystem."
	clientTools := CodexFallbackTools()
	_, calls := CompileAgentToolCalls(msgs, denial, nil, clientTools, "")
	if len(calls) == 0 {
		t.Fatal("expected bootstrap tool_calls on denial for codebase analysis")
	}
}

func TestPreemptiveExploreWithoutNotion(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "coba analisa codebase disini"}}
	calls := PreemptiveAgentToolCalls(msgs, CodexFallbackTools())
	if len(calls) == 0 {
		t.Fatal("expected preemptive tool_calls before Notion call")
	}
	names := ToolCallNames(calls)
	if len(names) == 0 || names[0] != "shell_command" {
		t.Fatalf("expected shell_command, got %v", names)
	}
}

func TestPreemptiveReadSpecificFile(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "analisa cmd\\server\\main.go"}}
	calls := PreemptiveAgentToolCalls(msgs, CodexFallbackTools())
	if len(calls) == 0 {
		t.Fatal("expected read tool call")
	}
	name := ToolCallNames(calls)[0]
	if name != "read_file" && name != "read_mcp_resource" {
		t.Fatalf("expected read tool, got %s", name)
	}
	fn, _ := calls[0]["function"].(map[string]any)
	if !strings.Contains(stringVal(fn["arguments"]), "main.go") {
		t.Fatalf("expected main.go path in args: %s", fn["arguments"])
	}
}

func TestScaffoldBlockedOnAnalyze(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "analisa cmd/server/main.go"}}
	denial := "Maaf, saya Notion AI"
	if calls := bootstrapScaffoldToolCalls(msgs, denial, CodexFallbackTools()); len(calls) != 0 {
		t.Fatalf("scaffold must not run on analyze task, got %v", ToolCallNames(calls))
	}
}

func TestExploreSkipsMCPTools(t *testing.T) {
	tools := fallbackTools([]string{"list_mcp_resources", "read_mcp_resource", "shell_command", "glob_file_search"})
	msgs := []ChatMessage{{Role: "user", Content: "analisa codebase"}}
	calls := buildExploreToolCalls(msgs, tools)
	if len(calls) == 0 {
		t.Fatal("expected explore tool call")
	}
	if name := ToolCallNames(calls)[0]; name == "list_mcp_resources" || name == "read_mcp_resource" {
		t.Fatalf("MCP tools must not be used for filesystem explore, got %s", name)
	}
}

func TestPreemptiveSkipsAfterToolHistory(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "analisa codebase"},
		{Role: "assistant", ToolCalls: []map[string]any{{
			"id": "call_1", "type": "function",
			"function": map[string]any{"name": "shell_command", "arguments": "{}"},
		}}},
		{Role: "tool", Content: "file listing...", ToolCallID: "call_1", Name: "shell_command"},
	}
	if calls := PreemptiveAgentToolCalls(msgs, CodexFallbackTools()); len(calls) != 0 {
		t.Fatalf("expected no preemptive after tool history, got %v", calls)
	}
}

func TestExploreUsesPowerShellOnWindows(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "analisa C:\\Users\\test\\poly-scan"}}
	calls := buildExploreToolCalls(msgs, CodexFallbackTools())
	if len(calls) == 0 {
		t.Fatal("expected tool call")
	}
	fn, _ := calls[0]["function"].(map[string]any)
	var args map[string]any
	_ = json.Unmarshal([]byte(stringVal(fn["arguments"])), &args)
	cmd := stringVal(args["command"])
	if !strings.Contains(cmd, "Get-ChildItem") {
		t.Fatalf("expected PowerShell list command, got %q", cmd)
	}
}

func TestSanitizeMCPToolToShell(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "analisa codebase"}}
	in := []map[string]any{makeToolCall("list_mcp_resources", map[string]any{"glob_pattern": "**/*"})}
	out := SanitizeExploreToolCalls(msgs, in, CodexFallbackTools())
	if len(out) == 0 || ToolCallNames(out)[0] != "shell_command" {
		t.Fatalf("expected shell_command, got %v", ToolCallNames(out))
	}
}

func TestShouldExploreBootstrapSkipsAfterListing(t *testing.T) {
	listing := strings.Repeat("src/main.go\n", 8)
	msgs := []ChatMessage{
		{Role: "user", Content: "analisa codebase"},
		{Role: "tool", Content: listing, Name: "shell_command"},
	}
	if ShouldExploreBootstrap(msgs) {
		t.Fatal("should not bootstrap after directory listing")
	}
	if calls := PreemptiveAgentToolCalls(msgs, CodexFallbackTools()); len(calls) != 0 {
		t.Fatalf("expected no preemptive calls, got %v", ToolCallNames(calls))
	}
}

func TestSanitizeDropsRepeatListAfterListing(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "analisa codebase"},
		{Role: "tool", Content: strings.Repeat("file.go\n", 10), Name: "shell_command"},
	}
	in := []map[string]any{makeToolCall("shell_command", map[string]any{"command": "Get-ChildItem -Force"})}
	if out := SanitizeExploreToolCalls(msgs, in, CodexFallbackTools()); len(out) != 0 {
		t.Fatalf("expected duplicate list commands dropped, got %v", ToolCallNames(out))
	}
}

func TestPreemptiveRetriesAfterEmptyMCPResult(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "analisa codebase"},
		{Role: "tool", Content: `{"resources": []}`, ToolCallID: "call_1", Name: "list_mcp_resources"},
	}
	calls := PreemptiveAgentToolCalls(msgs, CodexFallbackTools())
	if len(calls) == 0 || ToolCallNames(calls)[0] != "shell_command" {
		t.Fatalf("expected shell_command retry after empty MCP result, got %v", ToolCallNames(calls))
	}
}

func TestCompileAgentShellFence(t *testing.T) {
	text := "Jalankan ini:\n```bash\nls -la\n```"
	clientTools := CursorFallbackTools()
	_, calls := CompileAgentToolCalls(nil, text, nil, clientTools, "")
	if len(calls) != 1 {
		t.Fatalf("expected shell compile, got %d", len(calls))
	}
}

func TestFileReadDetectedWithoutPathInOutput(t *testing.T) {
	source := "package main\n\nimport (\n\t\"fmt\"\n\t\"net/http\"\n)\n\nfunc main() {\n\thttp.ListenAndServe(\":8080\", nil)\n}\n"
	msgs := []ChatMessage{
		{Role: "user", Content: "analisa cmd\\server\\main.go"},
		{Role: "assistant", ToolCalls: []map[string]any{{
			"id": "call_1", "type": "function",
			"function": map[string]any{
				"name": "shell_command",
				"arguments": `{"command":"Get-Content -Raw \"cmd\\server\\main.go\""}`,
			},
		}}},
		{Role: "tool", Content: source, ToolCallID: "call_1", Name: "shell_command"},
	}
	if !conversationHasFileReadResult(msgs, "cmd\\server\\main.go") {
		t.Fatal("expected file read detected from source code after read tool call")
	}
	if calls := PreemptiveAgentToolCalls(msgs, CodexFallbackTools()); len(calls) != 0 {
		t.Fatalf("expected no preemptive read after successful read, got %v", ToolCallNames(calls))
	}
}

func TestPreemptiveReadSkipsWhenPending(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "analisa cmd\\server\\main.go"}}
	pending := func(path string) bool { return path == "cmd\\server\\main.go" }
	if _, ok := shouldPreemptiveRead(msgs, pending); ok {
		t.Fatal("expected preemptive read skipped while read is pending")
	}
	if !AwaitingFileReadContent(msgs, pending) {
		t.Fatal("expected awaiting file read content while pending")
	}
}

func TestCacheAndEnrichFileRead(t *testing.T) {
	source := strings.Repeat("package main\nfunc main() {}\n", 10)
	msgs := []ChatMessage{
		{Role: "user", Content: "analisa cmd/server/main.go"},
		{Role: "assistant", ToolCalls: []map[string]any{{
			"id": "call_1", "type": "function",
			"function": map[string]any{
				"name": "shell_command",
				"arguments": `{"command":"Get-Content -Raw cmd/server/main.go"}`,
			},
		}}},
		{Role: "tool", Content: source, Name: "shell_command"},
	}
	cache := CacheFileReadsFromMessages(msgs)
	if len(cache) == 0 {
		t.Fatal("expected cached file read")
	}
	enriched := EnrichMessagesWithCachedRead(
		[]ChatMessage{{Role: "user", Content: "analisa cmd/server/main.go"}},
		"cmd/server/main.go",
		source,
	)
	if len(enriched) < 3 {
		t.Fatalf("expected enriched messages, got %d", len(enriched))
	}
}

func TestSkipCodexBootstrapPrompt(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "system", Content: "You are Codex CLI with terminal and filesystem access. Use tool_calls and run_terminal."},
		{Role: "user", Content: "analisa codebase"},
	}
	system, _, toolsActive, ideAgent, _, err := PrepareChatInput(msgs, CodexFallbackTools(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !toolsActive || !ideAgent {
		t.Fatal("expected active tools/ide agent")
	}
	if strings.Contains(system, "Codex CLI") {
		t.Fatalf("client bootstrap prompt should be filtered: %q", system)
	}
	if strings.Contains(system, "function-calling channel") {
		t.Fatalf("tool schemas must not be injected into Notion system prompt: %q", system)
	}
}