// Package tools is the entire interface between the model and your machine.
// It can ask for what is defined here and nothing else.
package tools

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ritik6559/cinch/internal/llm/openai"
)

type Tools struct {
	root string
}

const (
	maxReadLines = 2000
	maxReadBytes = 64 * 1024
	maxLineBytes = 1024 * 1024
	maxLineOut   = 2000
)

func New(root string) (*Tools, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return &Tools{root: abs}, nil
}

func (t *Tools) Root() string {
	return t.root
}

func (t *Tools) Definitions() []openai.Tool {
	return []openai.Tool{
		{
			Type: "function",
			Name: "read_file",
			Description: "Read a text file. Output is line-numbered and capped; if it is truncated the result says so and gives the offset to continue from. " +
				"Line numbers are display only — never include them in edit_file arguments.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to the file, e.g. internal/agent/agent.go",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "Number of lines to skip. Use the value suggested by a truncated read.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum lines to return. Defaults to 2000.",
					},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			Type:        "function",
			Name:        "write_file",
			Description: "Write content to a file at a relative path. Creates the file (and missing parent directories) or overwrites it.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to the file, e.g. internal/agent/agent.go",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Full contents to write to the file.",
					},
				},
				"required":             []string{"path", "content"},
				"additionalProperties": false,
			},
		},
		{
			Type:        "function",
			Name:        "edit_file",
			Description: "Replace an exact string in a file. old_string must match the file exactly, including indentation, and must be unique unless replace_all is true.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to the file, e.g. internal/agent/agent.go",
					},
					"old_string": map[string]any{
						"type":        "string",
						"description": "Exact text to replace, including indentation. Must be unique in the file unless replace_all is set.",
					},
					"new_string": map[string]any{
						"type":        "string",
						"description": "Replacement text.",
					},
					"replace_all": map[string]any{
						"type":        "boolean",
						"description": "Replace every occurrence of old_string. Defaults to false.",
					},
				},
				"required":             []string{"path", "old_string", "new_string"},
				"additionalProperties": false,
			},
		},
		{
			Type:        "function",
			Name:        "list_files",
			Description: "List files and directories. Directories end with a slash.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dir": map[string]any{
						"type":        "string",
						"description": "Relative directory. Defaults to the current one.",
					},
				},
				"required":             []string{},
				"additionalProperties": false,
			},
		},
	}
}

func (t *Tools) Run(name, arguments string) string {
	var args struct {
		Path       string `json:"path"`
		Dir        string `json:"dir"`
		Content    string `json:"content"`
		Offset     int    `json:"offset"`
		Limit      int    `json:"limit"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "error: bad arguments: " + err.Error()
	}

	switch name {
	case "read_file":
		return t.read_file(args.Path, args.Offset, args.Limit)

	case "write_file":
		abs, err := t.resolve(args.Path)
		if err != nil {
			return "error: " + err.Error()
		}

		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return "error: " + err.Error()
		}
		if err := os.WriteFile(abs, []byte(args.Content), 0o644); err != nil {
			return "error: " + err.Error()
		}
		return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path)

	case "edit_file":
		return t.editFile(args.Path, args.OldString, args.NewString, args.ReplaceAll)

	case "list_files":
		dir := args.Dir
		if dir == "" {
			dir = "."
		}
		abs, err := t.resolve(dir)
		if err != nil {
			return "error: " + err.Error()
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			return "error: " + err.Error()
		}
		var names []string
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() {
				n += "/"
			}
			names = append(names, n)
		}
		return strings.Join(names, "\n")
	}
	return "error: unknown tool " + name
}

func (t *Tools) read_file(path string, offset, limit int) string {
	abs, err := t.resolve(path)
	if err != nil {
		return "error: " + err.Error()
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "error: " + err.Error()
	}
	if info.IsDir() {
		return "error: " + path + " is a directory, use list_files"
	}

	f, err := os.Open(abs)
	if err != nil {
		return "error: " + err.Error()
	}
	defer f.Close()

	r := bufio.NewReader(f)

	// A NUL byte in the first 512 is the heuristic git uses for "binary".
	// Returning raw bytes would put thousands of junk tokens in the transcript.
	if head, _ := r.Peek(512); bytes.IndexByte(head, 0) >= 0 {
		return fmt.Sprintf("error: %s looks like a binary file (%s)",
			path, byteCount(int(info.Size())))
	}

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > maxReadLines {
		limit = maxReadLines
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	var (
		out       strings.Builder
		lineNo    int
		emitted   int
		bytesOut  int
		truncated bool
	)

	for scanner.Scan() {
		lineNo++
		if lineNo <= offset {
			continue
		}

		if emitted == limit || bytesOut > maxReadBytes {
			truncated = true
			break
		}

		line := scanner.Text()
		if len(line) > maxLineOut {
			// Back off to a rune boundary so we don't emit half a character.
			cut := maxLineOut
			for cut > 0 && !utf8.RuneStart(line[cut]) {
				cut--
			}
			line = line[:cut] + "… [line truncated]"
		}
		fmt.Fprintf(&out, "%6d\t%s\n", lineNo, line)
		bytesOut += len(line)
		emitted++
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Sprintf("error: %s has a line longer than %s; it is probably minified or generated — edit_file can still match an exact substring of it",
				path, byteCount(maxLineBytes))
		}
		return "error: " + err.Error()
	}

	if emitted == 0 {
		if offset > 0 {
			return fmt.Sprintf("no lines at offset %d in %s (file has %d lines)", offset, path, lineNo)
		}
		return "(empty file)"
	}

	if truncated {
		fmt.Fprintf(&out, "\n[truncated: showing lines %d-%d, file is %s. Continue with offset=%d]\n",
			offset+1, offset+emitted, byteCount(int(info.Size())), offset+emitted)
	}
	return out.String()
}

func (t *Tools) editFile(path, oldStr, newStr string, replaceAll bool) string {
	abs, err := t.resolve(path)
	if err != nil {
		return "error: " + err.Error()
	}
	if oldStr == "" {
		return "error: old_string must not be empty"
	}
	if oldStr == newStr {
		return "error: old_string and new_string must differ"
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "error: " + err.Error()
	}
	n := strings.Count(string(b), oldStr)
	switch {
	case n == 0:
		return "error: old_string not found in " + path
	case n > 1 && !replaceAll:
		return fmt.Sprintf("error: old_string appears %d times in %s; add surrounding context to make it unique or set replace_all", n, path)
	}
	count := 1
	if replaceAll {
		count = -1
	}
	updated := strings.Replace(string(b), oldStr, newStr, count)
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return "error: " + err.Error()
	}
	return fmt.Sprintf("replaced %d occurrence(s) in %s", n, path)
}

var mutating = map[string]bool{
	"write_file": true,
	"edit_file":  true,
}

func (t *Tools) NeedsApproval(name string) bool {
	return mutating[name]
}

func Summary(name, arguments string) string {
	var args struct {
		Path       string `json:"path"`
		Dir        string `json:"dir"`
		Content    string `json:"content"`
		Offset     int    `json:"offset"`
		Limit      int    `json:"limit"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return name
	}
	switch name {
	case "read_file":
		if args.Offset > 0 {
			return fmt.Sprintf("read_file: %s (from line %d)", args.Path, args.Offset+1)
		}
		return "read_file: " + args.Path

	case "write_file":
		return fmt.Sprintf("write file %s (%s)", args.Path, byteCount(len(args.Content)))

	case "edit_file":
		return fmt.Sprintf("edit file %s: replace %s with %s", args.Path, snippet(args.OldString), snippet(args.NewString))

	case "list_files":
		dir := args.Dir
		if dir == "" {
			dir = "."
		}
		return "list files in " + dir
	}
	return "error: unknown tool " + name
}

func byteCount(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

func snippet(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	if r := []rune(first); len(r) > 60 {
		first = string(r[:60]) + "…"
	}
	if extra := strings.Count(s, "\n"); extra > 0 {
		return fmt.Sprintf("%q (+%d lines)", first, extra)
	}
	return fmt.Sprintf("%q", first)
}
