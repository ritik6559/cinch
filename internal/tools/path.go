package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var secretFiles = map[string]bool{
	".env":       true,
	".env.local": true,
	".netrc":     true,
	".npmrc":     true,
}

func (t *Tools) resolve(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	if filepath.IsAbs(path) || os.IsPathSeparator(path[0]) || isWindowsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}

	abs := filepath.Clean(filepath.Join(t.root, path))
	if abs != t.root && !strings.HasPrefix(abs, t.root+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the workspace: %s", path)
	}
	if secretFiles[filepath.Base(abs)] {
		return "", fmt.Errorf("refusing to touch %s", filepath.Base(abs))
	}
	return abs, nil
}

// isWindowsAbs reports whether a path uses Windows absolute form: a drive
// letter such as "C:\dir", or a UNC network path such as "\\server\share".
//
// This check is deliberately independent of the operating system we run on.
func isWindowsAbs(p string) bool {
	if len(p) >= 2 && p[1] == ':' {
		return true
	}
	return strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//")
}
