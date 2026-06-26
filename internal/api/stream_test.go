package api

import (
	"net/http/httptest"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func TestWantsStreamDefaultFalse(t *testing.T) {
	req := chatRequest{}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if wantsStream(&req, r, false, false) {
		t.Fatal("expected default stream=false (OpenAI REST) when field omitted")
	}
}

func TestWantsStreamRespectsExplicitFalseForTools(t *testing.T) {
	req := chatRequest{Stream: boolPtr(false)}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if wantsStream(&req, r, true, false) {
		t.Fatal("expected stream=false when client explicitly disables streaming")
	}
}

func TestWantsStreamDefaultFalseForTools(t *testing.T) {
	req := chatRequest{}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if wantsStream(&req, r, true, false) {
		t.Fatal("expected default stream=false for tool requests unless stream:true")
	}
}

func TestWantsStreamExplicitTrue(t *testing.T) {
	req := chatRequest{Stream: boolPtr(true)}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if !wantsStream(&req, r, false, false) {
		t.Fatal("expected stream=true when explicitly set")
	}
}

func TestWantsStreamExplicitFalseChatOnly(t *testing.T) {
	req := chatRequest{Stream: boolPtr(false)}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if wantsStream(&req, r, false, false) {
		t.Fatal("expected stream=false when explicitly set for plain chat")
	}
}

func TestWantsStreamAcceptHeader(t *testing.T) {
	req := chatRequest{}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("Accept", "text/event-stream")
	if !wantsStream(&req, r, false, false) {
		t.Fatal("expected stream from Accept header when stream field omitted")
	}
}