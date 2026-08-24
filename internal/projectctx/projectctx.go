package projectctx

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	FileName = "AGENTS.md"
	maxBytes = 32 * 1024
)

func Load(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	text := strings.TrimSpace(string(data))
	if len(text) <= maxBytes {
		return text, nil
	}

	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + fmt.Sprintf("\n\n[truncated: %s is larger than %d KB]", FileName, maxBytes/1024), nil 
}

func Wrap(base, instructions string) string {
	if strings.TrimSpace(instructions) == "" {
		return base
	}

	return base + "\n\n" + `Project instructions
The repository provides the instructions below in ` + FileName + `. Follow them
for anything about this codebase: its conventions, its commands, its layout.
Where they conflict with the rules above, the rules above win.

` + instructions
}