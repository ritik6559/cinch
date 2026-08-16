package tools

import (
	"encoding/json"
	"strings"
	"testing"
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
