package tui

import "testing"

func TestSummarise(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		result string
		failed bool
		want   string
	}{
		{"empty", "grep", "", false, "no output"},
		{"whitespace only", "bash", "  \n\t ", false, "no output"},

		{"grep none", "grep", "no matches", false, "no matches"},
		{"grep one", "grep", "a.go:1: x", false, "1 match"},
		{"grep many", "grep", "a.go:1: x\nb.go:2: y", false, "2 matches"},

		{"glob none", "glob", "no files matched", false, "no files"},
		{"glob some", "glob", "a.go\nb.go\nc.go", false, "3 files"},

		{"read one line", "read_file", "package main", false, "1 line"},
		{"read many", "read_file", "a\nb\nc\nd", false, "4 lines"},

		{"list", "list_files", "a\nb", false, "2 entries"},

		{"bash with status", "bash", "exit status 1\nFAIL\nfoo", false, "exit status 1 · 2 lines"},
		{"bash without status", "bash", "hello", false, "1 line"},

		{"unknown tool flattens", "mystery", "one\ntwo", false, "one two"},

		{"failure shows first line", "bash", "error: boom\nmore", true, "boom …"},
		{"failure single line", "grep", "error: bad pattern", true, "bad pattern"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarise(tt.tool, tt.result, tt.failed); got != tt.want {
				t.Errorf("summarise(%q, %q, %v) = %q, want %q",
					tt.tool, tt.result, tt.failed, got, tt.want)
			}
		})
	}
}

func TestComma(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{999999, "999,999"},
		{1234567, "1,234,567"},
	}

	for _, tt := range tests {
		if got := comma(tt.n); got != tt.want {
			t.Errorf("comma(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestCompactTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0k"},
		{12400, "12.4k"},
		{1_500_000, "1.5M"},
	}

	for _, tt := range tests {
		if got := compactTokens(tt.n); got != tt.want {
			t.Errorf("compactTokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

// clip counts runes, not bytes: cutting a multi-byte character in half would
// print a replacement glyph.
func TestClipIsRuneSafe(t *testing.T) {
	if got := clip("héllo wörld", 5); got != "héllo…" {
		t.Errorf("clip = %q, want %q", got, "héllo…")
	}
	if got := clip("short", 20); got != "short" {
		t.Errorf("clip should leave short strings alone, got %q", got)
	}
	if got := clip("exact", 5); got != "exact" {
		t.Errorf("clip should not add an ellipsis at exactly max, got %q", got)
	}
}

func TestToolArgsStripsTheToolName(t *testing.T) {
	tests := []struct {
		summary string
		tool    string
		want    string
	}{
		{"run: go test ./...", "bash", "go test ./..."},
		{"grep maxSteps", "grep", "maxSteps"},
		{"write file main.go", "write_file", "main.go"},
		{"list files in internal", "list_files", "internal"},
		{"something else", "glob", "something else"},
	}

	for _, tt := range tests {
		if got := toolArgs(tt.summary, tt.tool); got != tt.want {
			t.Errorf("toolArgs(%q, %q) = %q, want %q", tt.summary, tt.tool, got, tt.want)
		}
	}
}
