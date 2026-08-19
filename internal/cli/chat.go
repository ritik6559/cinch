package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ritik6559/cinch/internal/agent"
	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/llm/openai"
	"github.com/ritik6559/cinch/internal/tools"
)

func chatCmd() *Command {
	return &Command{
		Name:    "chat",
		Summary: "Start an interactive session (this is the default)",
		Usage:   "cinch chat",
		Run:     runChat,
	}
}

func runChat(ctx context.Context, env *Env, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}
	ts, err := tools.New(root)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(env.Stdin)

	always := map[string]bool{}

	approve := func(tool, summary string) bool {
		if always[tool] {
			return true
		}
		fmt.Fprintf(env.Stdout, "\nallow %s? [y/N/a] ", summary)
		if !scanner.Scan() {
			return false // end of input means deny, never allow
		}
		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "y", "yes":
			return true
		case "a", "always":
			always[tool] = true
			return true
		default:
			return false
		}
	}

	a := agent.New(openai.New(cfg.APIKey, cfg.Model), ts, approve, env.Stdout)

	fmt.Fprintln(env.Stdout, "cinch — ask about the files in this directory. ctrl-c to quit.")

	for {
		fmt.Fprint(env.Stdout, "\nyou: ")
		if !scanner.Scan() {
			return scanner.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if err := a.Run(ctx, line); err != nil {
			// Ctrl-C during a turn: stop the whole program, not just this turn.
			if errors.Is(err, context.Canceled) {
				return err
			}
			fmt.Fprintf(env.Stderr, "error: %v\n", err)
		}

		fmt.Fprintln(env.Stdout, usageLine(a.TurnUsage(), a.Usage()))
	}
}

func usageLine(turn, session openai.Usage) string {
	var b strings.Builder

	fmt.Fprintf(&b, "  %s in", comma(turn.InputTokens))
	if turn.CachedTokens > 0 {
		fmt.Fprintf(&b, " (%s cached)", comma(turn.CachedTokens))
	}

	fmt.Fprintf(&b, " · %s out", comma(turn.OutputTokens))
	if turn.ReasoningTokens > 0 {
		fmt.Fprintf(&b, " (%s thinking)", comma(turn.ReasoningTokens))
	}

	fmt.Fprintf(&b, " · session %s in", comma(session.InputTokens))
	return b.String()
}

func comma(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}

	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
