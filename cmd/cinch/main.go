package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ritik6559/cinch/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	err := cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)

	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "cinch:", err)
	}

	stop()
	os.Exit(cli.ExitCode(err))
}
