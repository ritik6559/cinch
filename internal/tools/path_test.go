package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	root := t.TempDir()
	ts, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		want    string // expected resolved path, empty if an error is expected
		wantErr bool
	}{
		{"inside", "internal/agent/agent.go", filepath.Join(root, "internal", "agent", "agent.go"), false},
		{"dot", ".", root, false},
		{"dotdot inside", "a/../b.txt", filepath.Join(root, "b.txt"), false},
		{"empty", "", "", true},
		{"absolute windows", `C:\Windows\system32\drivers\etc\hosts`, "", true},
		{"absolute windows fwd slash", "C:/Windows/system32", "", true},
		{"absolute unix", "/etc/passwd", "", true},
		{"absolute unc", `\\server\share\secret.txt`, "", true},
		{"traversal", "../../../etc/passwd", "", true},
		{"nested traversal", "sub/../../../escape.txt", "", true},
		{"hidden traversal", "ok/../../outside.txt", "", true},
		{"secret env", "config/.env", "", true},
		{"secret env local", ".env.local", "", true},
		{"secret netrc", "auth/.netrc", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ts.resolve(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolve(%q) = %q, want error", tc.path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve(%q) unexpected error: %v", tc.path, err)
			}
			if got != tc.want {
				t.Fatalf("resolve(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// edit_file must not be able to modify a file outside the workspace root.
func TestEditFileConfined(t *testing.T) {
	outside := t.TempDir()
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	victim := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(victim, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]string{
		"path":       victim,
		"old_string": "original",
		"new_string": "pwned",
	})
	_ = ts.Run("edit_file", string(args))

	got, _ := os.ReadFile(victim)
	if string(got) != "original" {
		t.Fatalf("edit_file escaped the workspace: victim now %q", got)
	}
}

// write_file must land inside the workspace root even when the process
// working directory is somewhere else.
func TestWriteFileConfined(t *testing.T) {
	root := t.TempDir()
	ts, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]string{"path": "sub/new.txt", "content": "hi"})
	out := ts.Run("write_file", string(args))

	if _, err := os.Stat(filepath.Join(root, "sub", "new.txt")); err != nil {
		t.Fatalf("write_file did not create the file inside the workspace: %v; tool said: %s", err, out)
	}
}

// read_file must refuse to read secrets even inside the workspace.
func TestReadFileSecretBlocked(t *testing.T) {
	root := t.TempDir()
	ts, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("API_KEY=x"), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]string{"path": ".env"})
	out := ts.Run("read_file", string(args))
	if want := "error: refusing to touch .env"; len(out) < len(want) || out[:len(want)] != want {
		t.Fatalf("read_file on .env = %q, want refusal", out)
	}
}
