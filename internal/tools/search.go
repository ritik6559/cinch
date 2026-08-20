package tools

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	maxGrepMatches = 100
	maxGrepLine    = 300
	grepTimeout    = 15 * time.Second
)

var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"bin":          true,
	".venv":        true,
	"__pycache__":  true,
}

func (t *Tools) grep(ctx context.Context, pattern, path, glob string, insensitive bool) string {
	if pattern == "" {
		return "error: pattern is required"
	}
	if path == "" {
		path = "."
	}
	if _, err := t.resolve(path); err != nil {
		return "error: " + err.Error()
	}

	ctx, cancel := context.WithTimeout(ctx, grepTimeout)
	defer cancel()

	if out, ok := t.ripgrep(ctx, pattern, path, glob, insensitive); ok {
		return out
	}
	return t.grepWalk(ctx, pattern, path, glob, insensitive)
}

func (t *Tools) ripgrep(ctx context.Context, pattern, path, glob string, insensitive bool) (string, bool) {
	bin, err := exec.LookPath("rg")
	if err != nil {
		return "", false
	}

	args := []string{"--line-number", "--no-heading", "--color", "never"}
	if insensitive {
		args = append(args, "--ignore-case")
	}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, "--", pattern, path)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = t.root

	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			switch exit.ExitCode() {
			case 1:
				return "no matches", true // rg's exit code for "found nothing"
			case 2:
				return "error: " + strings.TrimSpace(string(exit.Stderr)), true
			}
		}
		return "", false
	}
	return formatMatches(strings.Split(strings.TrimRight(string(out), "\n"), "\n")), true
}

func (t *Tools) grepWalk(ctx context.Context, pattern, path, glob string, insensitive bool) string {
	if insensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "error: bad pattern" + err.Error()
	}

	root, err := t.resolve(path)
	if err != nil {
		return "error: " + err.Error()
	}

	var matches []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
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
		if glob != "" {
			if ok, _ := filepath.Match(glob, d.Name()); !ok {
				return nil
			}
		}

		rel, err := filepath.Rel(t.root, p)
		if err != nil {
			return nil
		}
		matches = append(matches, matchFile(p, filepath.ToSlash(rel), re)...)
		if len(matches) >= maxGrepMatches {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.SkipAll) {
		return "error: " + err.Error()
	}
	return formatMatches(matches)
}

func matchFile(abs, rel string, re *regexp.Regexp) []string {
	f, err := os.Open(abs)
	if err != nil {
		return nil
	}
	defer f.Close()

	r := bufio.NewReader(f)
	if head, _ := r.Peek(512); bytes.IndexByte(head, 0) >= 0 {
		return nil // binary
	}

	var out []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	for n := 1; scanner.Scan(); n++ {
		if re.MatchString(scanner.Text()) {
			out = append(out, fmt.Sprintf("%s:%d:%s", rel, n, scanner.Text()))
			if len(out) >= maxGrepMatches {
				return out
			}
		}
	}

	if err := scanner.Err(); err != nil {
		reason := err.Error()
		if errors.Is(err, bufio.ErrTooLong) {
			reason = "line too long; probably minified or generated"
		}
		out = append(out, fmt.Sprintf("%s: skipped: %s", rel, reason))
	}
	return out
}

func formatMatches(lines []string) string {
	var out []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		if len(line) > maxGrepLine {
			line = line[:maxGrepLine] + "..."
		}
		out = append(out, line)
		if len(out) == maxGrepMatches {
			out = append(out, fmt.Sprintf(
				"[truncated at %d matches: narrow the pattern, or set path or glob]", maxGrepMatches))
			break
		}
	}
	if len(out) == 0 {
		return "no matches"
	}
	return strings.Join(out, "\n")
}
