package tools

import (
	"strings"
	"testing"
)

func TestPrepareChatInput(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "Hello"}}
	system, prompt, toolsActive, ideAgent, _, err := PrepareChatInput(msgs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if prompt == "" || system == "" {
		t.Fatalf("empty system/prompt: %q / %q", system, prompt)
	}
	if toolsActive || ideAgent {
		t.Fatal("expected no tools")
	}
}

func TestExtractLastUserMessage(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "siapa presiden pertama indonesia"},
		{Role: "assistant", Content: "Ir Soekarno"},
		{Role: "user", Content: "siapa wakilnya"},
	}
	if got := ExtractLastUserMessage(msgs); got != "siapa wakilnya" {
		t.Fatalf("got %q", got)
	}
}

func TestAllSessionKeysOverlap(t *testing.T) {
	msgs1 := []ChatMessage{{Role: "user", Content: "siapa presiden pertama indonesia"}}
	msgs2 := []ChatMessage{
		{Role: "user", Content: "siapa presiden pertama indonesia"},
		{Role: "assistant", Content: "Ir Soekarno"},
		{Role: "user", Content: "siapa wakilnya"},
	}
	keys1 := AllSessionKeys("", "sk-test", msgs1)
	keys2 := AllSessionKeys("", "sk-test", msgs2)
	if len(keys1) == 0 || len(keys2) == 0 {
		t.Fatal("expected session keys")
	}
	overlap := false
	set1 := map[string]bool{}
	for _, k := range keys1 {
		set1[k] = true
	}
	for _, k := range keys2 {
		if set1[k] {
			overlap = true
			break
		}
	}
	if !overlap {
		t.Fatalf("expected overlapping keys: %v vs %v", keys1, keys2)
	}
}

func TestSessionKeyStickySingleMessage(t *testing.T) {
	turn1 := []ChatMessage{{Role: "user", Content: "siapa presiden pertama indonesia"}}
	turn2 := []ChatMessage{{Role: "user", Content: "siapa wakilnya"}}
	k1 := SessionKeyFromMessages("", "sk-test", turn1)
	k2 := SessionKeyFromMessages("", "sk-test", turn2)
	if k1 != k2 || !strings.HasPrefix(k1, "sticky:") {
		t.Fatalf("expected sticky session, got %q and %q", k1, k2)
	}
}

func TestParseAssistantOutputJSON(t *testing.T) {
	text := `{"content": null, "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "Shell", "arguments": "{}"}}]}`
	content, calls := ParseAssistantOutput(text)
	if content != "" {
		t.Fatalf("content: %q", content)
	}
	if len(calls) != 1 {
		t.Fatalf("calls: %d", len(calls))
	}
}

func TestLooksLikeToolDenial(t *testing.T) {
	if !LooksLikeToolDenial("I'm Notion AI and can't access your local filesystem") {
		t.Fatal("expected denial")
	}
}