package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ritik6559/cinch/internal/llm"
)

const Version = 1

const maxTitle = 60

var validID = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{4}$`)

type Session struct {
	Version   int           `json:"version"`
	ID        string        `json:"id"`
	Created   time.Time     `json:"created"`
	Updated   time.Time     `json:"updated"`
	Workspace string        `json:"workspace"`
	Provider  string        `json:"provider"`
	Model     string        `json:"model"`
	Title     string        `json:"title"`
	Usage     llm.Usage     `json:"usage"`
	Messages  []llm.Message `json:"messages"`
}

func New(workspace, provider, model string) *Session {
	now := time.Now()
	return &Session{
		Version:   Version,
		ID:        NewID(now),
		Created:   now,
		Updated:   now,
		Workspace: workspace,
		Provider:  provider,
		Model:     model,
	}
}

func NewID(t time.Time) string {
	var suffix [2]byte
	rand.Read(suffix[:])
	return t.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(suffix[:])
}

func (s *Session) SetTitle(prompt string) {
	if s.Title != "" {
		return
	}

	title := strings.Join(strings.Fields(prompt), " ")
	if r := []rune(title); len(r) > maxTitle {
		title = strings.TrimSpace(string(r[:maxTitle])) + "…"
	}
	s.Title = title
}

func (s *Session) Turns() int {
	n := 0
	for _, m := range s.Messages {
		if m.Role == llm.RoleUser && m.TextContent() != "" {
			n++
		}
	}
	return n
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cinch", "sessions"), nil
}

func path(id string) (string, error) {
	if !validID.MatchString(id) {
		return "", fmt.Errorf("invalid session id %q", id)
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

func (s *Session) Save() error {
	s.Updated = time.Now()

	final, err := path(s.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding session: %w", err)
	}

	temp := final + ".tmp"

	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, final); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}

func Load(id string) (*Session, error) {
	p, err := path(id)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("session %s is damaged: %w", id, err)
	}
	if s.Version > Version {
		return nil, fmt.Errorf("session %s was written by a newer cinch (format %d, this build understands %d)",
			id, s.Version, Version)
	}
	return &s, nil
}

func List() ([]*Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []*Session
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if s, err := Load(strings.TrimSuffix(name, ".json")); err == nil {
			out = append(out, s)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, nil
}

func Latest() (*Session, error) {
	all, err := List()
	if err != nil || len(all) == 0 {
		return nil, err
	}
	return all[0], nil
}
