//go:build linux

package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Landlock cannot be undone for the life of a process, so the real test has to
// happen in a child: confining the test binary itself would break every test
// that ran after it.
//
// The child is this same binary, re-run with an environment variable that sends
// it down the confinement path instead of the test suite.
const (
	childEnv   = "CINCH_TEST_CONFINE_ROOT"
	childProbe = "CINCH_TEST_CONFINE_PROBE"
)

func TestMain(m *testing.M) {
	root := os.Getenv(childEnv)
	if root == "" {
		os.Exit(m.Run())
	}

	if err := Confine(root); err != nil {
		os.Stderr.WriteString("confine failed: " + err.Error())
		os.Exit(3)
	}

	// Exit 0 if the probe path can be read, 1 if the kernel refused.
	if _, err := os.ReadFile(os.Getenv(childProbe)); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// readable runs a child that confines itself to root and then tries to read
// probe, and reports whether the read succeeded.
func readable(t *testing.T, root, probe string) bool {
	t.Helper()

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), childEnv+"="+root, childProbe+"="+probe)

	out, err := cmd.CombinedOutput()
	if err == nil {
		return true
	}

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("child failed to run: %v\n%s", err, out)
	}
	if exit.ExitCode() == 3 {
		t.Fatalf("child could not confine itself: %s", out)
	}
	return false
}

func TestConfine(t *testing.T) {
	if _, ok := Support(); !ok {
		t.Skip("this kernel has no Landlock support")
	}

	root := t.TempDir()
	inside := filepath.Join(root, "inside.txt")
	if err := os.WriteFile(inside, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !readable(t, root, inside) {
		t.Error("a file inside the workspace should still be readable")
	}
	if readable(t, root, outside) {
		t.Error("a file outside the workspace was still readable — nothing was enforced")
	}
}

// The point of confinement: credentials in the home directory become
// unreachable even to a command the user approved.
func TestConfineHidesHomeDotfiles(t *testing.T) {
	if _, ok := Support(); !ok {
		t.Skip("this kernel has no Landlock support")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}

	secret := filepath.Join(home, ".cinch-test-secret")
	if err := os.WriteFile(secret, []byte("key"), 0o600); err != nil {
		t.Skipf("cannot write to the home directory: %v", err)
	}
	t.Cleanup(func() { os.Remove(secret) })

	if readable(t, t.TempDir(), secret) {
		t.Error("a dotfile in the home directory was still readable")
	}
}

func TestSupportReportsAVersion(t *testing.T) {
	detail, ok := Support()
	if detail == "" {
		t.Fatal("Support() gave no explanation either way")
	}
	if ok && !strings.Contains(detail, "landlock") {
		t.Errorf("Support() = %q, want it to name landlock", detail)
	}
}
