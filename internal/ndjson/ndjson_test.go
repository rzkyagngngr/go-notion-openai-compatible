package ndjson

import "testing"

func TestCleanNotionOutputTextStripsSearchPreamble(t *testing.T) {
	in := "Let me search for that.\n\n# Hello World"
	out := CleanNotionOutputText(in)
	if out != "# Hello World" {
		t.Fatalf("got %q", out)
	}
}

func TestStreamParserPatchText(t *testing.T) {
	p := NewStreamParser()
	lines := []string{
		`{"type":"patch-start","data":{"s":[{"type":"agent-reply","value":[{"type":"text","content":"Hi"}]}]}}`,
	}
	for _, line := range lines {
		if err := p.FeedLine(line); err != nil {
			t.Fatal(err)
		}
	}
	result := p.Finalize()
	if result.Text != "Hi" {
		t.Fatalf("text: %q", result.Text)
	}
}

func TestStreamParserToolUse(t *testing.T) {
	p := NewStreamParser()
	line := `{"type":"patch","v":[{"o":"a","p":"/s/0/value/-","v":{"type":"tool_use","name":"Shell","id":"call_1","input":{"command":"ls"}}}]}`
	if err := p.FeedLine(`{"type":"patch-start","data":{"s":[]}}`); err != nil {
		t.Fatal(err)
	}
	if err := p.FeedLine(line); err != nil {
		t.Fatal(err)
	}
	result := p.Finalize()
	if len(result.ToolCalls) == 0 {
		t.Fatal("expected tool calls")
	}
	fn, _ := result.ToolCalls[0]["function"].(map[string]any)
	if fn["name"] != "Shell" {
		t.Fatalf("tool name: %v", fn["name"])
	}
}