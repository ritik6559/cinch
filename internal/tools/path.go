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
	if filepath.IsAbs(path) || os.IsPathSeparator(path[0]) {
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