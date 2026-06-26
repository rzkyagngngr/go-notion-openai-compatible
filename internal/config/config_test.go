package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	s := Load()
	if s.Host == "" || s.Port == 0 {
		t.Fatalf("invalid settings: %+v", s)
	}
	if s.SessionFile == "" {
		t.Fatal("expected session file path")
	}
}