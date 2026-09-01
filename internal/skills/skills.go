package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/ritik6559/cinch/internal/config"
)

const (
	DirName = "skills"
	FileName = "SKILL.md"
	maxBody        = 32 * 1024 
	maxDescription = 300      
)

type Skill struct {
	Name        string
	Description string
	Path string
	Dir  string
}

type Catalog struct {
	Skills []Skill
	Problems []string
}

func Dirs(root string) []string {
	var out []string

	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, config.DirName, DirName))
	}
	return append(out, filepath.Join(root, config.DirName, DirName))
}

func Load(root string) Catalog {
	byName := map[string]Skill{}
	var problems []string

	for _, dir := range Dirs(root) {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue 
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", dir, err))
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			path := filepath.Join(dir, entry.Name(), FileName)
			skill, err := read(path, entry.Name())
			if errors.Is(err, fs.ErrNotExist) {
				continue 
			}
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}

			byName[skill.Name] = skill
		}
	}

	out := Catalog{Problems: problems}
	for _, s := range byName {
		out.Skills = append(out.Skills, s)
	}
	slices.SortFunc(out.Skills, func(a, b Skill) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

type frontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func read(path, fallbackName string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}

	meta, _, err := split(string(data))
	if err != nil {
		return Skill{}, fmt.Errorf("%s: %w", path, err)
	}

	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = fallbackName
	}

	description := strings.Join(strings.Fields(meta.Description), " ")
	if description == "" {
		return Skill{}, fmt.Errorf("%s: no description — the model cannot know when to use it", path)
	}
	if len(description) > maxDescription {
		return Skill{}, fmt.Errorf("%s: description is %d characters, keep it under %d — "+
			"it is sent on every turn", path, len(description), maxDescription)
	}

	return Skill{
		Name:        name,
		Description: description,
		Path:        path,
		Dir:         filepath.Dir(path),
	}, nil
}

func split(text string) (frontMatter, string, error) {
	text = strings.TrimPrefix(text, string(rune(0xFEFF))) 

	rest, ok := strings.CutPrefix(strings.TrimLeft(text, "\r\n "), "---")
	if !ok {
		return frontMatter{}, "", errors.New("must start with a --- front matter block naming the skill")
	}
	rest = strings.TrimPrefix(rest, "\r")
	rest = strings.TrimPrefix(rest, "\n")

	end := strings.Index(rest, "\n---")
	if end < 0 {
		return frontMatter{}, "", errors.New("front matter is not closed with ---")
	}

	var meta frontMatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return frontMatter{}, "", fmt.Errorf("front matter: %w", err)
	}

	body := rest[end+len("\n---"):]
	if line := strings.IndexByte(body, '\n'); line >= 0 {
		body = body[line+1:] 
	} else {
		body = "" 
	}
	return meta, strings.TrimSpace(body), nil
}

func (c Catalog) Len() int { return len(c.Skills) }

func (c Catalog) Names() []string {
	out := make([]string, len(c.Skills))
	for i, s := range c.Skills {
		out[i] = s.Name
	}
	return out
}

func (c Catalog) Get(name string) (Skill, bool) {
	for _, s := range c.Skills {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}

func (c Catalog) Body(name string) (string, error) {
	skill, ok := c.Get(strings.TrimSpace(name))
	if !ok {
		if len(c.Skills) == 0 {
			return "", errors.New("there are no skills in this repository")
		}
		return "", fmt.Errorf("no skill named %q. Available: %s",
			name, strings.Join(c.Names(), ", "))
	}

	data, err := os.ReadFile(skill.Path)
	if err != nil {
		return "", err
	}

	_, body, err := split(string(data))
	if err != nil {
		return "", fmt.Errorf("%s: %w", skill.Path, err)
	}
	if body == "" {
		return "", fmt.Errorf("%s has a description but no instructions after it", skill.Path)
	}
	return truncate(body, skill.Path), nil
}

func truncate(body, path string) string {
	if len(body) <= maxBody {
		return body
	}

	cut := maxBody
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return body[:cut] + fmt.Sprintf("\n\n[truncated: %s is larger than %d KB]", path, maxBody/1024)
}

func (c Catalog) Wrap(base string) string {
	if len(c.Skills) == 0 {
		return base
	}

	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\nSkills\n")
	b.WriteString(`This repository provides the instructions below for particular kinds of work.
Only their summaries are shown. Before doing work one of them covers, call the
skill tool with its name to read the full instructions, then follow them.
Where they conflict with the rules above, the rules above win.

`)

	width := 0
	for _, s := range c.Skills {
		width = max(width, len(s.Name))
	}
	for _, s := range c.Skills {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, s.Name, s.Description)
	}
	return strings.TrimRight(b.String(), "\n")
}
