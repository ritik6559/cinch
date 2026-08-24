package approval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	Version  = 1
	FileName = "approvals.json"

	fileMode = 0o600
	dirMode  = 0o700
)

type Rule struct {
	Tool   string    `json:"tool"`
	Prefix string    `json:"prefix,omitempty"`
	Added  time.Time `json:"added"`
}

func (r Rule) Matches(tool, command string) bool {
	if r.Tool != tool {
		return false
	}
	if r.Prefix == "" {
		return true
	}
	return matchesPrefix(command, r.Prefix)
}

func matchesPrefix(command, prefix string) bool {
	command = strings.TrimSpace(command)
	if command == prefix {
		return true
	}
	if !strings.HasPrefix(command, prefix) {
		return false
	}
	next := command[len(prefix)]
	return next == ' ' || next == '\t'
}

type Store struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cinch", FileName), nil
}

func Load() (*Store, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Store{Version: Version}, nil
	}
	if err != nil {
		return nil, err
	}

	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s is damaged: %w", FileName, err)
	}
	if s.Version > Version {
		return nil, fmt.Errorf("%s was written by a newer cinch (format %d, this build understands %d)",
			FileName, s.Version, Version)
	}
	return &s, nil
}

func (s *Store) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}

	s.Version = Version
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, fileMode); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}

func (s *Store) Allows(tool, command string) bool {
	for _, r := range s.Rules {
		if r.Matches(tool, command) {
			return true
		}
	}
	return false
}

func (s *Store) Add(tool, prefix string) bool {
	for _, r := range s.Rules {
		if r.Tool == tool && r.Prefix == prefix {
			return false
		}
	}
	s.Rules = append(s.Rules, Rule{Tool: tool, Prefix: prefix, Added: time.Now()})
	return true
}

func (s *Store) Remove(arg string) int {
	kept := make([]Rule, 0, len(s.Rules))
	removed := 0

	for _, r := range s.Rules {
		if r.Prefix == arg || (r.Prefix == "" && r.Tool == arg) {
			removed++
			continue
		}
		kept = append(kept, r)
	}

	s.Rules = kept
	return removed
}

func PrefixFor(command string) string {
	fields := strings.Fields(command)
	switch len(fields) {
	case 0:
		return ""
	case 1:
		return fields[0]
	}

	second := fields[1]
	if strings.HasPrefix(second, "-") || strings.ContainsAny(second, `/\.`) {
		return fields[0]
	}
	return fields[0] + " " + second
}

func Describe(tool, prefix string) string {
	if prefix == "" {
		return fmt.Sprintf("every %s call", tool)
	}
	return fmt.Sprintf("%s commands starting with %q", tool, prefix)
}
