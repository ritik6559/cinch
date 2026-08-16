package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ritik6559/cinch/internal/agent"
	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/llm/openai"
	"github.com/ritik6559/cinch/internal/tools"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinch:", err)
		os.Exit(1)
	}

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinch: ", err)
		os.Exit(1)
	}

	ts, err := tools.New(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinch: ", err)
		os.Exit(1)
	}
	scanner := bufio.NewScanner(os.Stdin)

	approve_always := map[string]bool{}

	approve := func(tool, summary string) bool {
		fmt.Printf("\nallow %s> [y/N/a] ", summary)
		if !scanner.Scan() {
			return false
		}
		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "y", "yes":
			return true
		case "a", "always":
			approve_always[tool] = true
			return true
		default:
			return false
		}
	}

	a := agent.New(openai.New(cfg.APIKey, cfg.Model), ts, func(tool, summary string) bool {
		return approve_always[tool] || approve(tool, summary)
	}, os.Stdout)
	ctx := context.Background()

	fmt.Println("cinch — ask about the files in this directory. ctrl-c to quit.")

	for {
		fmt.Print("\nyou: ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			}
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := a.Run(ctx, line); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
}
