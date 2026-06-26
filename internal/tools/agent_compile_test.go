package tools

import (
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
	clientTools := CursorFallbackTools()
	_, calls := CompileAgentToolCalls(msgs, denial, nil, clientTools, "")
	if len(calls) == 0 {
		t.Fatal("expected bootstrap tool_calls on denial for codebase analysis")
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
	system, _, toolsActive, ideAgent, _, err := PrepareChatInput(msgs, CursorFallbackTools(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !toolsActive || !ideAgent {
		t.Fatal("expected active tools/ide agent")
	}
	if strings.Contains(system, "Codex CLI") {
		t.Fatalf("client bootstrap prompt should be filtered: %q", system)
	}
	if !strings.Contains(system, "coding agent") {
		t.Fatal("expected injected agent instruction")
	}
}