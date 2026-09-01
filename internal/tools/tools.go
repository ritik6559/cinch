package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/skills"
)

type Tools struct {
	root   string
	skills skills.Catalog
}

const maxLineBytes = 1024 * 1024

type Option func(*Tools)

func WithSkills(c skills.Catalog) Option {
	return func(t *Tools) { t.skills = c }
}

func New(root string, opts ...Option) (*Tools, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}

	t := &Tools{root: abs}
	for _, opt := range opts {
		opt(t)
	}
	return t, nil
}

func (t *Tools) Root() string {
	return t.root
}

func (t *Tools) Definitions() []llm.ToolDef {
	defs := []llm.ToolDef{
		{
			Name: "read_file",
			Description: "Read a text file. Output is line-numbered and capped; if it is truncated the result says so and gives the offset to continue from. " +
				"Line numbers are display only — never include them in edit_file arguments.",
			Schema: map[string]any{
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
			Name:        "write_file",
			Description: "Write content to a file at a relative path. Creates the file (and missing parent directories) or overwrites it.",
			Schema: map[string]any{
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
			Name:        "edit_file",
			Description: "Replace an exact string in a file. old_string must match the file exactly, including indentation, and must be unique unless replace_all is true.",
			Schema: map[string]any{
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
			Name:        "list_files",
			Description: "List files and directories. Directories end with a slash.",
			Schema: map[string]any{
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
		{
			Name: "grep",
			Description: "Search file contents with a regular expression. Returns path:line:text for each match. " +
				"Use this to locate code instead of reading files speculatively.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Regular expression, RE2 syntax.",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Relative directory or file to search. Defaults to the workspace root.",
					},
					"glob": map[string]any{
						"type":        "string",
						"description": "Only search files whose name matches this pattern, e.g. *.go",
					},
					"case_insensitive": map[string]any{
						"type":        "boolean",
						"description": "Ignore case. Defaults to false.",
					},
				},
				"required":             []string{"pattern"},
				"additionalProperties": false,
			},
		},
		{
			Name: "glob",
			Description: "Find files by name pattern. * matches within one name, ** matches any number of directories, ? matches one character. " +
				"Returns matching paths, one per line. Use this to find files by name; use grep to find them by content.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Name pattern, for example **/*_test.go or internal/**/config.go",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Relative directory to search under. Defaults to the workspace root.",
					},
				},
				"required":             []string{"pattern"},
				"additionalProperties": false,
			},
		},
		{
			Name: "bash",
			Description: "Run a shell command in the workspace root and return its combined output and exit status. " +
				"Use it to build, run tests, and inspect git. Commands are POSIX shell, never PowerShell, on every platform. " +
				"Prefer read_file, edit_file and grep for working with files: they are safer and cheaper than shelling out.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The shell command, for example: go test ./...",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Seconds to allow. Defaults to 120, maximum 600.",
					},
				},
				"required":             []string{"command"},
				"additionalProperties": false,
			},
		},
	}

	if t.skills.Len() > 0 {
		defs = append(defs, llm.ToolDef{
			Name: "skill",
			Description: "Read the full instructions for one of this repository's skills. " +
				"The system prompt lists them with a one-line summary each; use this to get " +
				"the rest before doing work that skill covers.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "The skill to read.",
						"enum":        t.skills.Names(),
					},
				},
				"required":             []string{"name"},
				"additionalProperties": false,
			},
		})
	}

	return defs
}

type toolArgs struct {
	Path            string `json:"path"`
	Dir             string `json:"dir"`
	Content         string `json:"content"`
	Offset          int    `json:"offset"`
	Limit           int    `json:"limit"`
	OldString       string `json:"old_string"`
	NewString       string `json:"new_string"`
	ReplaceAll      bool   `json:"replace_all"`
	Pattern         string `json:"pattern"`
	Glob            string `json:"glob"`
	CaseInsensitive bool   `json:"case_insensitive"`
	Command         string `json:"command"`
	Timeout         int    `json:"timeout"`
	Name            string `json:"name"`
}

func (t *Tools) Run(ctx context.Context, name, arguments string) string {
	var args toolArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "error: bad arguments: " + err.Error()
	}

	switch name {
	case "read_file":
		return t.readFile(args.Path, args.Offset, args.Limit)
	case "write_file":
		return t.writeFile(args.Path, args.Content)
	case "edit_file":
		return t.editFile(args.Path, args.OldString, args.NewString, args.ReplaceAll)
	case "list_files":
		return t.listFiles(args.Dir)
	case "grep":
		return t.grep(ctx, args.Pattern, args.Path, args.Glob, args.CaseInsensitive)
	case "glob":
		return t.glob(args.Pattern, args.Path)
	case "bash":
		return t.bash(ctx, args.Command, args.Timeout)
	case "skill":
		body, err := t.skills.Body(args.Name)
		if err != nil {
			return "error: " + err.Error()
		}
		return body
	}
	return "error: unknown tool " + name
}

var mutating = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"bash":       true,
}

func (t *Tools) NeedsApproval(name string) bool {
	return mutating[name]
}

func Summary(name, arguments string) string {
	var args toolArgs
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

	case "grep":
		if args.Path != "" {
			return fmt.Sprintf("grep %q in %s", args.Pattern, args.Path)
		}
		return fmt.Sprintf("grep %q", args.Pattern)

	case "glob":
		if args.Path != "" {
			return fmt.Sprintf("glob %s in %s", args.Pattern, args.Path)
		}
		return "glob " + args.Pattern

	case "bash":
		return "run: " + firstLine(args.Command, 100)

	case "skill":
		return "skill: " + args.Name
	}
	return "error: unknown tool " + name
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)

	line, rest, found := strings.Cut(s, "\n")
	if r := []rune(line); len(r) > max {
		line = string(r[:max]) + "…"
	}
	if found && strings.TrimSpace(rest) != "" {
		line += " (+more lines)"
	}
	return line
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

func CommandOf(arguments string) string {
	var args toolArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ""
	}
	return args.Command
}
