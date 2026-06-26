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