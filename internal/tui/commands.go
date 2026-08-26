package tui

import "strings"

type command struct {
	Name string
	Args string
	Desc string
}

var commands = []command{
	{Name: "model", Args: "[id]", Desc: "Change the model"},
	{Name: "effort", Args: "[level]", Desc: "Change how hard the model thinks"},
	{Name: "compact", Desc: "Shrink the conversation now"},
	{Name: "sessions", Desc: "Switch to a saved session"},
	{Name: "resume", Args: "<id>", Desc: "Resume a session by id"},
	{Name: "approvals", Desc: "Review and remove saved approvals"},
	{Name: "cost", Desc: "Token usage for this session"},
	{Name: "clear", Desc: "Start a new conversation"},
	{Name: "help", Desc: "List these commands"},
	{Name: "quit", Desc: "Exit cinch"},
}

func isCommand(line string) bool {
	return strings.HasPrefix(line, "/")
}

func splitCommand(line string) (name, args string) {
	line = strings.TrimPrefix(strings.TrimSpace(line), "/")
	name, args, _ = strings.Cut(line, " ")
	return strings.ToLower(name), strings.TrimSpace(args)
}

func matchCommands(line string) []command {
	if !isCommand(line) {
		return nil
	}

	name, _, hasArgs := strings.Cut(strings.TrimPrefix(line, "/"), " ")
	if hasArgs {
		return nil
	}

	name = strings.ToLower(name)
	out := make([]command, 0, len(commands))
	for _, c := range commands {
		if strings.HasPrefix(c.Name, name) {
			out = append(out, c)
		}
	}
	return out
}

func lookupCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.Name == name {
			return c, true
		}
	}
	return command{}, false
}

func completion(c command) string {
	if c.Args != "" {
		return "/" + c.Name + " "
	}
	return "/" + c.Name
}
