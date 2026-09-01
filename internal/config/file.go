package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/sandbox"
)

const (
	DirName  = ".cinch"
	FileName = "config.yaml"
)

type Trust int

const (
	Trusted Trust = iota
	Untrusted
)

type File struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	BaseURL   string `yaml:"base_url"`
	Effort    string `yaml:"effort"`
	CompactAt *int   `yaml:"compact_at"`
	Sandbox   string `yaml:"sandbox"`
}

func ReadFile(path string) (File, error) {
	handle, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, err
	}
	defer handle.Close()

	decoder := yaml.NewDecoder(handle)

	decoder.KnownFields(true)

	var f File
	if err := decoder.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return File{}, nil
		}
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func (f File) Apply(c *Config, trust Trust, path string) error {
	if trust == Untrusted {
		for _, field := range []struct{ name, value string }{
			{"provider", f.Provider},
			{"base_url", f.BaseURL},
		} {
			if field.value != "" {
				return fmt.Errorf("%s: %s cannot be set by a project file — it decides "+
					"where your API key is sent. Set it in the environment instead",
					path, field.name)
			}
		}
	}

	if f.Provider != "" {
		c.Provider = f.Provider
	}
	if f.Model != "" {
		c.Model = f.Model
	}
	if f.BaseURL != "" {
		c.BaseURL = f.BaseURL
	}

	if f.Effort != "" {
		if !llm.ValidEffort(f.Effort) {
			return fmt.Errorf("%s: effort must be one of %s, got %q",
				path, strings.Join(llm.Efforts, ", "), f.Effort)
		}
		c.Effort = f.Effort
	}

	if f.CompactAt != nil {
		if *f.CompactAt < 0 {
			return fmt.Errorf("%s: compact_at cannot be negative, got %d", path, *f.CompactAt)
		}
		c.CompactAt = *f.CompactAt
	}

	if f.Sandbox != "" {
		if !sandbox.ValidMode(f.Sandbox) {
			return fmt.Errorf("%s: sandbox must be one of %s, got %q",
				path, strings.Join(sandbox.Modes, ", "), f.Sandbox)
		}
		mode := sandbox.Mode(f.Sandbox)

		if trust == Untrusted && sandboxRank(mode) < sandboxRank(c.Sandbox) {
			return fmt.Errorf("%s: a project file cannot lower the sandbox from %q to %q",
				path, c.Sandbox, mode)
		}
		c.Sandbox = mode
	}

	return nil
}

func sandboxRank(m sandbox.Mode) int {
	switch m {
	case sandbox.ModeOff:
		return 0
	case sandbox.ModeStrict:
		return 2
	case sandbox.ModeConfined:
		return 3
	default:
		return 1
	}
}

type Source struct {
	Path  string
	Trust Trust
}

func Sources(root string) []Source {
	var out []Source

	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, Source{filepath.Join(home, DirName, FileName), Trusted})
	}
	return append(out, Source{filepath.Join(root, DirName, FileName), Untrusted})
}

// Found reports which of the settings files actually exist, for `cinch doctor`.
func Found(root string) []string {
	var out []string

	for _, src := range Sources(root) {
		if _, err := os.Stat(src.Path); err == nil {
			out = append(out, src.Path)
		}
	}
	return out
}

// envFor maps a settings key to the variables that override it.
var envFor = map[string][]string{
	"provider":   {"CINCH_PROVIDER"},
	"model":      {"CINCH_MODEL", "OPENAI_MODEL"},
	"base_url":   {"CINCH_BASE_URL"},
	"effort":     {"CINCH_EFFORT"},
	"compact_at": {"CINCH_COMPACT_AT"},
	"sandbox":    {"CINCH_SANDBOX"},
}

// keysSet names the settings this file actually mentions.
func (f File) keysSet() []string {
	var out []string

	for key, on := range map[string]bool{
		"provider":   f.Provider != "",
		"model":      f.Model != "",
		"base_url":   f.BaseURL != "",
		"effort":     f.Effort != "",
		"compact_at": f.CompactAt != nil,
		"sandbox":    f.Sandbox != "",
	} {
		if on {
			out = append(out, key)
		}
	}
	return out
}

// Shadowed lists settings a file asks for that the environment overrides.
//
// The environment winning is deliberate, but it is invisible: a setting written
// carefully into a file and then quietly ignored looks like a broken feature
// rather than a rule working as intended. `.env` counts here, because by the
// time anything reads it, it is part of the environment.
func Shadowed(root string) []string {
	set := map[string]bool{}

	for _, src := range Sources(root) {
		f, err := ReadFile(src.Path)
		if err != nil {
			continue // a broken file is reported by loading it, not here
		}
		for _, key := range f.keysSet() {
			set[key] = true
		}
	}

	var out []string
	for _, key := range slices.Sorted(maps.Keys(set)) {
		for _, name := range envFor[key] {
			if os.Getenv(name) != "" {
				out = append(out, key+" overridden by "+name)
				break
			}
		}
	}
	return out
}
