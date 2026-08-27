package sandbox

import (
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestModeDecide(t *testing.T) {
	tests := []struct {
		mode    Mode
		command string
		want    Action
	}{
		// off restores the behaviour cinch had before the policy existed.
		{ModeOff, "ls", Ask},
		{ModeOff, "go test", Ask},
		{ModeOff, "rm -rf /", Ask},

		{ModePolicy, "ls", Allow},
		{ModePolicy, "go test", Ask},
		{ModePolicy, "rm -rf /", Deny},

		// strict keeps the refusals and asks about everything else.
		{ModeStrict, "ls", Ask},
		{ModeStrict, "go test", Ask},
		{ModeStrict, "rm -rf /", Deny},

		// confined judges exactly as policy does; the kernel does the rest.
		{ModeConfined, "ls", Allow},
		{ModeConfined, "go test", Ask},
		{ModeConfined, "rm -rf /", Deny},

		// An unset mode must be the default, not the weakest one.
		{"", "ls", Allow},
		{"", "rm -rf /", Deny},
	}

	for _, tt := range tests {
		if got := tt.mode.Decide(tt.command).Action; got != tt.want {
			t.Errorf("Mode(%q).Decide(%q) = %s, want %s", tt.mode, tt.command, got, tt.want)
		}
	}
}

func TestOnlyConfinedWantsTheKernel(t *testing.T) {
	for _, m := range []Mode{ModeOff, ModePolicy, ModeStrict, ""} {
		if m.Confines() {
			t.Errorf("Mode(%q).Confines() = true, want false", m)
		}
	}
	if !ModeConfined.Confines() {
		t.Error("ModeConfined.Confines() = false, want true")
	}
}

func TestValidMode(t *testing.T) {
	for _, name := range Modes {
		if !ValidMode(name) {
			t.Errorf("ValidMode(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "on", "POLICY", "sandbox", "confine"} {
		if ValidMode(name) {
			t.Errorf("ValidMode(%q) = true, want false", name)
		}
	}
	if !slices.Contains(Modes, "confined") {
		t.Error("Modes should list confined")
	}
}

// Support must always explain itself, so doctor can print something useful
// whatever the platform.
func TestSupportAlwaysExplains(t *testing.T) {
	detail, ok := Support()

	if detail == "" {
		t.Fatal("Support() returned no detail")
	}
	if runtime.GOOS != "linux" {
		if ok {
			t.Errorf("Support() claims enforcement on %s", runtime.GOOS)
		}
		if !strings.Contains(detail, runtime.GOOS) {
			t.Errorf("Support() = %q, want it to name the platform", detail)
		}
	}
}

// Asking for confinement where there is none has to fail loudly. Degrading to
// nothing would leave someone believing they were protected.
func TestConfineFailsWhereItCannotWork(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("linux has landlock; covered by confine_linux_test.go")
	}

	err := Confine(t.TempDir())
	if err == nil {
		t.Fatalf("Confine succeeded on %s, where there is no sandbox", runtime.GOOS)
	}
	if !strings.Contains(err.Error(), runtime.GOOS) {
		t.Errorf("error = %q, want it to name the platform", err)
	}
}
