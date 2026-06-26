package client

import "testing"

func TestShouldReleaseStreamBufferAgentFast(t *testing.T) {
	if !shouldReleaseStreamBuffer("thinking.", true) {
		t.Fatal("expected early release for agent mode")
	}
}

func TestShouldReleaseStreamBufferChatWaits(t *testing.T) {
	if shouldReleaseStreamBuffer("short", false) {
		t.Fatal("expected short chat text to wait")
	}
	if !shouldReleaseStreamBuffer("this is a longer chunk of plain chat text for streaming release", false) {
		t.Fatal("expected release after length threshold")
	}
}