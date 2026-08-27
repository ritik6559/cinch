package sandbox

import (
	"path/filepath"
	"slices"
	"strings"
)

var opaqueWords = map[string]bool{
	"eval": true, "exec": true, "source": true, ".": true,
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true,
}

func Split(command string) (segments []string, reason string) {
	var (
		out   []string
		cur   strings.Builder
		quote byte
	)

	note := func(what string) {
		if reason == "" {
			reason = what
		}
	}

	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}

	for i := 0; i < len(command); i++ {
		c := command[i]

		if quote != 0 {
			cur.WriteByte(c)
			switch {
			case c == '\\' && quote == '"' && i+1 < len(command):
				i++
				cur.WriteByte(command[i])
			case c == quote:
				quote = 0
			case c == '`' && quote == '"':
				note("command substitution")
			case c == '$' && quote == '"' && i+1 < len(command) && command[i+1] == '(':
				note("command substitution")
			}
			continue
		}

		switch c {
		case '\'', '"':
			quote = c
			cur.WriteByte(c)

		case '\\':
			cur.WriteByte(c)
			if i+1 < len(command) {
				i++
				cur.WriteByte(command[i])
			}

		case '`':
			note("command substitution")
			cur.WriteByte(c)

		case '$':
			if i+1 < len(command) && (command[i+1] == '(' || command[i+1] == '{') {
				note("command substitution")
			}
			cur.WriteByte(c)

		case '<', '>':
			cur.WriteByte(c)
			for i+1 < len(command) && (command[i+1] == '&' || command[i+1] == '>') {
				i++
				cur.WriteByte(command[i])
			}

		case '&':
			if i+1 < len(command) && command[i+1] == '>' { // `&>file`
				cur.WriteByte(c)
				i++
				cur.WriteByte(command[i])
				continue
			}
			if i+1 < len(command) && command[i+1] == '&' {
				i++
			}
			flush()

		case '|':
			if i+1 < len(command) && command[i+1] == '|' {
				i++
			}
			flush()

		case ';', '\n':
			flush()

		default:
			cur.WriteByte(c)
		}
	}
	flush()

	if quote != 0 {
		note("unbalanced quote")
	}

	for _, segment := range out {
		if word := firstWord(segment); opaqueWords[word] {
			note(word + " runs a command chosen at runtime")
		}
		if strings.HasPrefix(segment, "$") {
			note("the command name comes from a variable")
		}
		if target, ok := redirectOutside(segment); ok {
			note("redirects to " + target)
		}
	}

	return out, reason
}

func firstWord(segment string) string {
	word, _, _ := strings.Cut(strings.TrimSpace(segment), " ")
	return strings.Trim(word, `"'`)
}

func redirectOutside(segment string) (string, bool) {
	fields := strings.Fields(segment)

	for i, field := range fields {
		op := strings.TrimLeft(field, "0123456789")
		if !strings.HasPrefix(op, ">") && !strings.HasPrefix(op, "<") {
			continue
		}

		target := strings.TrimLeft(op, "<>&")
		if target == "" && i+1 < len(fields) {
			target = fields[i+1]
		}
		if outsideWorkspace(target) {
			return target, true
		}
	}
	return "", false
}

func outsideWorkspace(target string) bool {
	target = strings.Trim(target, `"'`)

	switch {
	case target == "":
		return false
	case strings.HasPrefix(target, "~"):
		return true
	case strings.HasPrefix(target, "/") || strings.HasPrefix(target, `\`):
		return true
	case filepath.IsAbs(target):
		return true
	case len(target) >= 2 && target[1] == ':': // C:\ on Windows
		return true
	}

	return slices.Contains(strings.Split(filepath.ToSlash(target), "/"), "..")
}
