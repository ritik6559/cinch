package version

import (
	"strings"
	"testing"
)

func TestDetailed(t *testing.T) {
	out := Detailed()

	if !strings.HasPrefix(out, "cinch ") {
		t.Errorf("output should start with %q, got %q", "cinch ", out)
	}
	if !strings.Contains(out, runtimeLine()) {
		t.Errorf("output should contain the Go version line, got %q", out)
	}
}

func runtimeLine() string { return "go" }
