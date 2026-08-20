package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultBashTimeout = 2 * time.Minute
	maxBashTimeout     = 10 * time.Minute
	maxBashOutput      = 30 * 1024
)

var secretEnvMarkers = []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL"}

func (t *Tools) bash(ctx context.Context, command string, timeoutSeconds int) string {
	if strings.TrimSpace(command) == "" {
		return "error: command is required"
	}

	shell, err := posixShell()
	if err != nil {
		return "error: " + err.Error()
	}

	timeout := defaultBashTimeout
	if timeoutSeconds > 0 {
		timeout = min(time.Duration(timeoutSeconds)*time.Second, maxBashTimeout)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = t.Root()
	cmd.Env = safeEnv()
	cmd.Stdin = nil

	out, runErr := cmd.CombinedOutput()
	text := truncateOutput(string(out))

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("error: command timed out after %s\n%s", timeout, text)
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return fmt.Sprintf("exit status %d\n%s", exitErr.ExitCode(), text)
	}
	if runErr != nil {
		return "error: " + runErr.Error()
	}
	if text == "" {
		return "(no output, exit status 0)"
	}
	return text
}

func posixShell() (string, error) {
	for _, name := range []string{"bash", "sh"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("no POSIX shell on PATH, install Git Bash")
}

func safeEnv() []string {
	parent := os.Environ()
	out := make([]string, 0, len(parent))

	for _, entry := range parent {
		name, _, _ := strings.Cut(entry, "=")
		if !isSecretEnv(name) {
			out = append(out, entry)
		}
	}
	return out
}

func isSecretEnv(name string) bool {
	upper := strings.ToUpper(name)
	for _, marker := range secretEnvMarkers {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func truncateOutput(s string) string {
	if len(s) <= maxBashOutput {
		return s
	}

	head := backToRuneStart(s, maxBashOutput*2/5)
	tail := backToRuneStart(s, len(s)-(maxBashOutput-head))

	return fmt.Sprintf("%s\n\n[... %d bytes omitted ...]\n\n%s",
		s[:head], tail-head, s[tail:])
}

func backToRuneStart(s string, i int) int {
	for i > 0 && i < len(s) && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}