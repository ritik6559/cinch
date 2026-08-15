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
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cinch:", err)
		os.Exit(1)
	}

	a := agent.New(openai.New(cfg.APIKey, cfg.Model), os.Stdout)
	ctx := context.Background()

	fmt.Println("cinch — ask about the files in this directory. ctrl-c to quit.")
	scanner := bufio.NewScanner(os.Stdin)

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