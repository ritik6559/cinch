// Package tools is the entire interface between the model and your machine.
// It can ask for what is defined here and nothing else.
package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ritik6559/cinch/internal/llm/openai"
)

func Definitions() []openai.Tool {
	return []openai.Tool{
		{
			Type:        "function",
			Name:        "read_file",
			Description: "Read the contents of a file at a relative path.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to the file, e.g. internal/agent/agent.go",
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

func Run(name, arguments string) string {
	var args struct {
		Path    string `json:"path"`
		Dir     string `json:"dir"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "error: bad arguments: " + err.Error()
	}

	switch name {
	case "read_file":
		b, err := os.ReadFile(args.Path)
		if err != nil {
			return "error: " + err.Error()
		}
		return string(b)

	case "write_file":
		if err := os.MkdirAll(filepath.Dir(args.Path), 0o755); err != nil {
			return "error: " + err.Error()
		}
		if err := os.WriteFile(args.Path, []byte(args.Content), 0o644); err != nil {
			return "error: " + err.Error()
		}
		return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path)

	case "list_files":
		dir := args.Dir
		if dir == "" {
			dir = "."
		}
		entries, err := os.ReadDir(dir)
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
