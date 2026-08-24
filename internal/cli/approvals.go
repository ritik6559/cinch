package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/ritik6559/cinch/internal/approval"
)

func approvalsCmd() *Command {
	return &Command{
		Name:    "approvals",
		Summary: "List or remove saved approvals",
		Usage: "cinch approvals\n" +
			"cinch approvals rm <prefix|tool>\n\n" +
			"Approvals are saved by answering s at an approval prompt.",
		Run: runApprovals,
	}
}

func runApprovals(ctx context.Context, env *Env, args []string) error {
	saved, err := approval.Load()
	if err != nil {
		return err
	}

	if len(args) > 0 {
		if args[0] != "rm" {
			return usagef("unknown argument %q. Use 'cinch approvals' or 'cinch approvals rm <prefix>'", args[0])
		}
		if len(args) < 2 {
			return usagef("rm needs something to remove. Run 'cinch approvals' to see them")
		}
		return removeApproval(env, saved, args[1])
	}

	return listApprovals(env, saved)
}

func listApprovals(env *Env, saved *approval.Store) error {
	if len(saved.Rules) == 0 {
		path, _ := approval.Path()
		fmt.Fprintf(env.Stdout, "no saved approvals (%s)\n", path)
		return nil
	}

	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	for _, r := range saved.Rules {
		what := r.Prefix
		if what == "" {
			what = "(every call)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Tool, what, r.Added.Format("2006-01-02"))
	}
	return tw.Flush()
}

func removeApproval(env *Env, saved *approval.Store, arg string) error {
	removed := saved.Remove(arg)
	if removed == 0 {
		return fmt.Errorf("no approval matches %q. Run 'cinch approvals' to see them", arg)
	}
	if err := saved.Save(); err != nil {
		return err
	}

	fmt.Fprintf(env.Stdout, "removed %d approval(s) matching %q\n", removed, arg)
	return nil
}
