package sessionrefresh

import "testing"

func TestIsStaleInferenceLine(t *testing.T) {
	for _, line := range []string{"[", "[]", " [ "} {
		if !IsStaleInferenceLine(line) {
			t.Fatalf("expected stale: %q", line)
		}
	}
	if IsStaleInferenceLine(`{"type":"patch"}`) {
		t.Fatal("patch should not be stale")
	}
}