package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const maxGlobResults = 200

func (t *Tools) glob(pattern, path string) string {
	if pattern == "" {
		return "error: pattern is required"
	}
	if path == "" {
		path = "."
	}

	re, err := globToRegexp(pattern)
	if err != nil {
		return "error: bad pattern: " + err.Error()
	}

	root, err := t.resolve(path)
	if err != nil {
		return "error: " + err.Error()
	}

	var matches []string
	truncated := false

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if secretFiles[d.Name()] {
			return nil
		}

		fromSearchRoot, err := filepath.Rel(root, p)
		if err != nil || !re.MatchString(filepath.ToSlash(fromSearchRoot)) {
			return nil
		}

		if len(matches) >= maxGlobResults {
			truncated = true
			return fs.SkipAll
		}

		rel, err := filepath.Rel(t.root, p)
		if err != nil {
			return nil
		}
		matches = append(matches, filepath.ToSlash(rel))
		return nil
	})
	if err != nil && !errors.Is(err, fs.SkipAll) {
		return "error: " + err.Error()
	}

	if len(matches) == 0 {
		return "no files match " + pattern
	}

	slices.Sort(matches)
	out := strings.Join(matches, "\n")
	if truncated {
		out += fmt.Sprintf("\n[truncated at %d files: narrow the pattern or set path]", maxGlobResults)
	}
	return out
}

// globToRegexp translates a shell-style pattern.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")

	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:[^/]+/)*")
					continue
				}
				b.WriteString(".*")
				continue
			}
			b.WriteString("[^/]*")

		case '?':
			b.WriteString("[^/]")

		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}

	b.WriteString("$")
	return regexp.Compile(b.String())
}
