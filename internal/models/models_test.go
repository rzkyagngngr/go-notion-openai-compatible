package models

import "testing"

func TestResolveModelAliases(t *testing.T) {
	got := ResolveModel("opus-4.8", "ambrosia-tart-high", nil)
	if got != "ambrosia-tart-high" {
		t.Fatalf("opus-4.8 -> %q", got)
	}
	got = ResolveModel("notion/gpt-4o", "ambrosia-tart-high", nil)
	if got != "ambrosia-tart-high" {
		t.Fatalf("notion/gpt-4o -> %q", got)
	}
}

func TestFriendlyAlias(t *testing.T) {
	if FriendlyAlias("Opus 4.8") != "opus-4.8" {
		t.Fatal("alias failed")
	}
}

func TestParseAvailableModels(t *testing.T) {
	raw := map[string]any{
		"models": []any{
			map[string]any{"modelMessage": "Opus 4.8", "model": "ambrosia-tart-high", "isDisabled": false},
		},
	}
	m := ParseAvailableModels(raw)
	if m["opus-4.8"] != "ambrosia-tart-high" {
		t.Fatalf("parse: %+v", m)
	}
}