package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/skills"
)

func skillsCmd() *Command {
	return &Command{
		Name:    "skills",
		Summary: "List the skills available here",
		Usage:   "cinch skills",
		Run:     runSkills,
	}
}

func runSkills(ctx context.Context, env *Env, args []string) error {
	if len(args) > 0 {
		return usagef("skills takes no arguments, got %q", args[0])
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}
	catalog := skills.Load(root)

	for _, problem := range catalog.Problems {
		fmt.Fprintf(env.Stderr, "warning: %s\n", problem)
	}

	if catalog.Len() == 0 {
		fmt.Fprintf(env.Stdout, "no skills. Add one at %s\n",
			filepath.Join(config.DirName, skills.DirName, "<name>", skills.FileName))
		return nil
	}

	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	for _, s := range catalog.Skills {
		fmt.Fprintf(tw, "%s\t%s\n", s.Name, s.Description)
	}
	return tw.Flush()
}
