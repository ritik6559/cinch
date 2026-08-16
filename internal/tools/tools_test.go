package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSummary(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"list_files", map[string]any{"dir": "internal"}, "list files in internal"},
		// dir omitted: Run defaults to ".", Summary must too.
		{"list_files", map[string]any{}, "list files in ."},
		{"read_file", map[string]any{"path": "a.go"}, "read_file: a.go"},
		// offset is lines skipped, so the first line shown is offset+1.
		{"read_file", map[string]any{"path": "a.go", "offset": 40}, "read_file: a.go (from line 41)"},
		{
			"edit_file",
			map[string]any{"path": "a.go", "old_string": "x", "new_string": "y"},
			`edit file a.go: replace "x" with "y"`,
		},
		{
			"edit_file",
			map[string]any{
				"path":       "a.go",
				"old_string": "func run() {\n\tbody()\n}\n",
				"new_string": "func run() error {\n\tbody()\n\treturn nil\n}\n",
			},
			`edit file a.go: replace "func run() {" (+3 lines) with "func run() error {" (+4 lines)`,
		},
		{
			// deletion: empty new_string, and a long single line gets truncated
			"edit_file",
			map[string]any{"path": "a.go", "old_string": strings.Repeat("x", 100), "new_string": ""},
			`edit file a.go: replace "` + strings.Repeat("x", 60) + `…" with ""`,
		},
	}

	for _, tc := range cases {
		args, err := json.Marshal(tc.args)
		if err != nil {
			t.Fatal(err)
		}
		if got := Summary(tc.name, string(args)); got != tc.want {
			t.Fatalf("Summary(%q, %s) = %q, want %q", tc.name, args, got, tc.want)
		}
	}
}

// A multi-line edit must not put the whole escaped blob in the approval line.
func TestSummaryEditStaysShort(t *testing.T) {
	args, _ := json.Marshal(map[string]string{
		"path":       "a.go",
		"old_string": strings.Repeat("line\n", 40),
		"new_string": strings.Repeat("LINE\n", 42),
	})
	got := Summary("edit_file", string(args))
	if len(got) > 200 {
		t.Fatalf("edit_file summary is %d chars, want a short approval line: %s", len(got), got)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runRead(t *testing.T, ts *Tools, args map[string]any) string {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return ts.Run("read_file", string(b))
}

// tenLines is 61 bytes: "line1\n"..."line9\n" (54) + "line10\n" (7).
func tenLines() string {
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	return b.String()
}

func TestReadFileNumbersLines(t *testing.T) {
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(ts.Root(), "a.txt"), "a\nb\nc")

	want := "     1\ta\n     2\tb\n     3\tc\n"
	if got := runRead(t, ts, map[string]any{"path": "a.txt"}); got != want {
		t.Fatalf("read_file = %q, want %q", got, want)
	}

	// CRLF input must not leak \r into the transcript.
	writeTestFile(t, filepath.Join(ts.Root(), "crlf.txt"), "a\r\nb\r\n")
	if got, want := runRead(t, ts, map[string]any{"path": "crlf.txt"}), "     1\ta\n     2\tb\n"; got != want {
		t.Fatalf("read_file on CRLF file = %q, want %q", got, want)
	}
}

func TestReadFileOffsetLimitTruncation(t *testing.T) {
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(ts.Root(), "ten.txt"), tenLines())

	// offset 4 skips lines 1-4; limit 3 stops after line 7; the footer must
	// name the real line range and the exact offset that continues after it.
	got := runRead(t, ts, map[string]any{"path": "ten.txt", "offset": 4, "limit": 3})
	want := "     5\tline5\n     6\tline6\n     7\tline7\n" +
		"\n[truncated: showing lines 5-7, file is 61 B. Continue with offset=7]\n"
	if got != want {
		t.Fatalf("read_file offset/limit = %q, want %q", got, want)
	}

	// A file of exactly `limit` lines ends on its own: no truncation footer.
	got = runRead(t, ts, map[string]any{"path": "ten.txt", "limit": 10})
	if strings.Contains(got, "[truncated") {
		t.Fatalf("exact-length read claims truncation: %q", got)
	}
	if !strings.Contains(got, "    10\tline10\n") {
		t.Fatalf("missing line 10: %q", got)
	}
}

func TestReadFileByteCap(t *testing.T) {
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// 80 lines of 999 bytes (Text() drops the \n): the 64 KB cap lets through
	// 66 lines (999*66 = 65934 first exceeds 65536) and then reports.
	content := strings.Repeat(strings.Repeat("a", 999)+"\n", 80)
	writeTestFile(t, filepath.Join(ts.Root(), "big.txt"), content)

	got := runRead(t, ts, map[string]any{"path": "big.txt"})
	if !strings.Contains(got, "[truncated: showing lines 1-66") {
		t.Fatalf("byte cap did not report lines 1-66: %q", strings.Split(got, "\n")[66])
	}
	if !strings.Contains(got, "Continue with offset=66]") {
		t.Fatalf("footer suggests the wrong continuation offset:\n%s", got[len(got)-120:])
	}
}

func TestReadFileEdgeCases(t *testing.T) {
	root := t.TempDir()
	ts, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "ten.txt"), tenLines())
	writeTestFile(t, filepath.Join(root, "empty.txt"), "")
	writeTestFile(t, filepath.Join(root, "blob.bin"), "ab\x00cd")
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"empty file", map[string]any{"path": "empty.txt"}, "(empty file)"},
		{"offset past EOF", map[string]any{"path": "ten.txt", "offset": 15},
			"no lines at offset 15 in ten.txt (file has 10 lines)"},
		{"directory", map[string]any{"path": "sub"},
			"error: sub is a directory, use list_files"},
	}
	for _, tc := range cases {
		if got := runRead(t, ts, tc.args); got != tc.want {
			t.Fatalf("%s: read_file = %q, want %q", tc.name, got, tc.want)
		}
	}

	if got := runRead(t, ts, map[string]any{"path": "blob.bin"}); !strings.Contains(got, "looks like a binary file") {
		t.Fatalf("read_file on binary = %q, want binary refusal", got)
	}
}

func TestReadFileLongLine(t *testing.T) {
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// 3001 bytes; byte 2000 falls mid-rune, so naive cutting emits invalid
	// UTF-8 that would arrive as mojibake in the transcript.
	writeTestFile(t, filepath.Join(ts.Root(), "wide.txt"), "x"+strings.Repeat("é", 1500))

	got := runRead(t, ts, map[string]any{"path": "wide.txt"})
	if !strings.Contains(got, "[line truncated]") {
		t.Fatalf("long line not marked as truncated: %q", got[:120])
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncated output is not valid UTF-8")
	}
}

func TestReadFileHugeLine(t *testing.T) {
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// One line beyond the scanner's 1 MB ceiling: minified/generated file.
	writeTestFile(t, filepath.Join(ts.Root(), "min.js"), strings.Repeat("a", maxLineBytes+100))

	got := runRead(t, ts, map[string]any{"path": "min.js"})
	if !strings.HasPrefix(got, "error:") || !strings.Contains(got, "longer than") {
		t.Fatalf("read_file on huge line = %q, want ErrTooLong error", got)
	}
}
