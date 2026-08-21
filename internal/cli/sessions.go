package cli

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/ritik6559/cinch/internal/session"
)

func sessionsCmd() *Command {
	return &Command{
		Name:    "sessions",
		Summary: "List saved sessions",
		Usage:   "cinch sessions\n\nResume one with: cinch --resume <id>",
		Run:     runSessions,
	}
}

func runSessions(ctx context.Context, env *Env, args []string) error {
	all, err := session.List()
	if err != nil {
		return err
	}

	if len(all) == 0 {
		dir, _ := session.Dir()
		fmt.Fprintf(env.Stdout, "no saved sessions yet (%s)\n", dir)
		return nil
	}

	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	for _, s := range all {
		fmt.Fprintf(tw, "%s\t%s\t%d turns\t%s\t%s\n",
			s.ID,
			ago(s.Updated),
			s.Turns(),
			comma(s.Usage.InputTokens)+" in",
			s.Title,
		)
	}
	return tw.Flush()
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
