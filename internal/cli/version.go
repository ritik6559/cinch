package cli

import (
	"context"
	"fmt"

	"github.com/ritik6559/cinch/internal/version"
)

func versionCmd() *Command {
	return &Command{
		Name:    "version",
		Summary: "Print version information",
		Usage:   "cinch version",
		Run: func(ctx context.Context, env *Env, args []string) error {
			fmt.Fprintln(env.Stdout, version.Detailed())
			return nil
		},
	}
}