package tools

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxReadLines = 2000
	maxReadBytes = 64 * 1024
	maxLineOut   = 2000
)

func (t *Tools) readFile(path string, offset, limit int) string {
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

func (t *Tools) writeFile(path, content string) string {
	abs, err := t.resolve(path)
	if err != nil {
		return "error: " + err.Error()
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "error: " + err.Error()
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "error: " + err.Error()
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), path)
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

func (t *Tools) listFiles(dir string) string {
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
