package sandbox

import (
	"slices"
	"strings"
)

type Action int

const (
	Allow Action = iota
	Ask
	Deny
)

func (a Action) String() string {
	switch a {
	case Allow:
		return "allow"
	case Ask:
		return "ask"
	case Deny:
		return "deny"
	}
	return "unknown"
}

type Verdict struct {
	Action Action
	Reason string
}

type Mode string

const (
	ModeOff      Mode = "off"
	ModePolicy   Mode = "policy"
	ModeStrict   Mode = "strict"
	ModeConfined Mode = "confined"
)

var Modes = []string{"off", "policy", "strict", "confined"}

func (m Mode) Confines() bool { return m == ModeConfined }

func ValidMode(s string) bool {
	return slices.Contains(Modes, s)
}

func (m Mode) Decide(command string) Verdict {
	switch m {
	case ModeOff:
		return Verdict{Ask, ""}

	case ModeStrict:
		if v := Decide(command); v.Action != Allow {
			return v
		}
		return Verdict{Ask, ""}

	default:
		return Decide(command)
	}
}

func Decide(command string) Verdict {
	if strings.TrimSpace(command) == "" {
		return Verdict{Ask, "empty command"}
	}
	if forkBomb(command) {
		return Verdict{Deny, "this is a fork bomb"}
	}

	segments, reason := Split(command)
	parsed := make([]Command, len(segments))
	for i, s := range segments {
		parsed[i] = parse(s)
	}

	for _, c := range parsed {
		if why := catastrophic(c); why != "" {
			return Verdict{Deny, why}
		}
	}
	if why := downloadsAndRuns(parsed); why != "" {
		return Verdict{Deny, why}
	}

	if reason != "" {
		return Verdict{Ask, reason}
	}
	if len(parsed) == 0 {
		return Verdict{Ask, "empty command"}
	}

	var reasons []string
	for _, c := range parsed {
		switch {
		case networkCommands[c.Name]:
			reasons = append(reasons, c.Name+" reaches the network")
		case !readOnlyCommand(c):
			reasons = append(reasons, c.Name+" can change things")
		default:
			if target, outside := escapingArgument(c); outside {
				reasons = append(reasons, c.Name+" reads "+target)
			}
		}
	}

	if len(reasons) == 0 {
		return Verdict{Allow, ""}
	}
	return Verdict{Ask, strings.Join(dedupe(reasons), " and ")}
}

type Command struct {
	Name string
	Args []string
}

func parse(segment string) Command {
	fields := strings.Fields(segment)
	if len(fields) == 0 {
		return Command{}
	}

	unquote := func(s string) string { return strings.Trim(s, `"'`) }
	c := Command{Name: unquote(fields[0])}
	for _, f := range fields[1:] {
		c.Args = append(c.Args, unquote(f))
	}
	return c
}

var readOnlyCommands = map[string]bool{
	"ls": true, "pwd": true, "cat": true, "head": true, "tail": true,
	"wc": true, "file": true, "stat": true, "du": true, "df": true,
	"echo": true, "printf": true, "date": true, "whoami": true, "uname": true,
	"which": true, "type": true, "tree": true, "basename": true, "dirname": true,
	"sort": true, "uniq": true, "cut": true, "grep": true, "rg": true,
	"true": true, "false": true, "hostname": true, "id": true, "env": true,
}

var readOnlySubcommands = map[string]map[string]bool{
	"git": {
		"status": true, "log": true, "diff": true, "show": true,
		"branch": true, "blame": true, "describe": true, "rev-parse": true,
	},
	"go":     {"version": true, "env": true, "list": true, "doc": true},
	"npm":    {"ls": true, "view": true, "outdated": true},
	"cargo":  {"tree": true},
	"docker": {"ps": true, "images": true},
}

var networkCommands = map[string]bool{
	"curl": true, "wget": true, "nc": true, "ncat": true, "netcat": true,
	"ssh": true, "scp": true, "sftp": true, "telnet": true, "ftp": true,
	"rsync": true,
}

var scriptRunners = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true,
	"python": true, "python3": true, "perl": true, "ruby": true, "node": true,
}

func readOnlyCommand(c Command) bool {
	if subs, ok := readOnlySubcommands[c.Name]; ok {
		for _, a := range c.Args {
			if strings.HasPrefix(a, "-") {
				continue
			}
			return subs[a]
		}
		return false
	}

	if !readOnlyCommands[c.Name] {
		return false
	}

	for _, a := range c.Args {
		if writingFlags[a] {
			return false
		}
	}
	return true
}

var writingFlags = map[string]bool{
	"-i": true, "--in-place": true, "-o": true, "--output": true,
	"--delete": true, "-exec": true, "-execdir": true, "-ok": true,
}

func escapingArgument(c Command) (string, bool) {
	for _, a := range c.Args {
		if strings.HasPrefix(a, "-") || !looksLikePath(a) {
			continue
		}
		if outsideWorkspace(a) {
			return a, true
		}
	}
	return "", false
}

func looksLikePath(a string) bool {
	if strings.Contains(a, "://") {
		return false
	}
	return strings.ContainsAny(a, `/\~`)
}

func catastrophic(c Command) string {
	switch {
	case c.Name == "rm":
		if target, ok := deletesSystemPath(c.Args); ok {
			return "rm would delete " + target
		}

	case strings.HasPrefix(c.Name, "mkfs"):
		return c.Name + " formats a filesystem"

	case c.Name == "dd":
		for _, a := range c.Args {
			if strings.HasPrefix(a, "of=/dev/") {
				return "dd would overwrite the raw device " + strings.TrimPrefix(a, "of=")
			}
		}

	case c.Name == "shutdown", c.Name == "reboot", c.Name == "halt", c.Name == "poweroff":
		return c.Name + " would stop the machine"

	case c.Name == "chmod", c.Name == "chown":
		if target, ok := deletesSystemPath(c.Args); ok {
			return c.Name + " would change permissions on " + target
		}
	}
	return ""
}

func deletesSystemPath(args []string) (string, bool) {
	recursive := false
	for _, a := range args {
		switch {
		case a == "--recursive", a == "-R":
			recursive = true
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--"):
			if strings.ContainsAny(a, "rR") {
				recursive = true
			}
		}
	}
	if !recursive {
		return "", false
	}

	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if rootLike(a) {
			return a, true
		}
	}
	return "", false
}

func rootLike(p string) bool {
	p = strings.Trim(p, `"'`)
	if p == "" {
		return false
	}

	p = strings.TrimSuffix(p, "*")
	p = strings.TrimRight(p, "/")

	if p == "" {
		return true
	}

	switch p {
	case "~", "$HOME", "${HOME}", "$env:USERPROFILE":
		return true
	}
	if strings.HasPrefix(p, "/") {
		return !strings.Contains(strings.Trim(p, "/"), "/")
	}
	return false
}

func downloadsAndRuns(commands []Command) string {
	fetched := ""

	for _, c := range commands {
		if networkCommands[c.Name] {
			fetched = c.Name
			continue
		}
		if fetched != "" && scriptRunners[c.Name] {
			return fetched + " into " + c.Name + " runs a script nobody here has read"
		}
	}
	return ""
}

func forkBomb(command string) bool {
	stripped := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' {
			return -1
		}
		return r
	}, command)

	return strings.Contains(stripped, ":(){:|:&};:") || strings.Contains(stripped, ":(){:|:&15};:")
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]

	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
