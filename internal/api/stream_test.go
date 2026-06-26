package api

import (
	"net/http/httptest"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func TestWantsStreamDefaultTrue(t *testing.T) {
	req := chatRequest{}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if !wantsStream(&req, r, false, false) {
		t.Fatal("expected default stream=true when field omitted")
	}
}

func TestWantsStreamForceForTools(t *testing.T) {
	req := chatRequest{Stream: boolPtr(false)}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if !wantsStream(&req, r, true, false) {
		t.Fatal("expected stream forced for toolsActive")
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
	req := chatRequest{Stream: boolPtr(false)}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("Accept", "text/event-stream")
	if !wantsStream(&req, r, false, false) {
		t.Fatal("expected stream from Accept header")
	}
}