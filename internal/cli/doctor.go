package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"text/tabwriter"

	"github.com/ritik6559/cinch/internal/approval"
	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/projectctx"
	"github.com/ritik6559/cinch/internal/provider"
	"github.com/ritik6559/cinch/internal/session"
	"github.com/ritik6559/cinch/internal/version"
)

func doctorCmd() *Command {
	return &Command{
		Name:    "doctor",
		Summary: "Check that the local setup is complete",
		Usage:   "cinch doctor",
		Run:     runDoctor,
	}
}

type status int

const (
	pass status = iota
	warn
	fail
)

func (s status) label() string {
	switch s {
	case pass:
		return "ok"
	case warn:
		return "warn"
	default:
		return "fail"
	}
}

type check struct {
	name   string
	status status
	detail string
}

func runDoctor(ctx context.Context, env *Env, args []string) error {
	checks := []check{
		{"cinch", pass, version.Short()},
		{"go", pass, fmt.Sprintf("%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)},
	}
	checks = append(checks, configChecks()...)
	checks = append(checks, sessionCheck())
	checks = append(checks, agentsCheck())
	checks = append(checks, approvalsCheck())
	checks = append(checks,
		binary("bash", "required by the bash tool"),
		binary("git", "how you review and undo the changes cinch makes"),
		binary("rg", "makes grep faster and skips files listed in .gitignore"),
	)

	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)

	failed := 0
	for _, c := range checks {
		if c.status == fail {
			failed++
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.status.label(), c.name, c.detail)
	}
	tw.Flush()

	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	return nil
}

func agentsCheck() check {
	root, err := os.Getwd()
	if err != nil {
		return check{projectctx.FileName, warn, err.Error()}
	}

	instructions, err := projectctx.Load(root)
	if err != nil {
		return check{projectctx.FileName, warn, err.Error()}
	}
	if instructions == "" {
		return check{projectctx.FileName, warn, "not found — this repository gives cinch no project-specific instructions"}
	}
	return check{projectctx.FileName, pass, fmt.Sprintf("%d bytes", len(instructions))}
}

func configChecks() []check {
	out := []check{{"workspace", pass, workingDir()}}

	cfg, err := config.Load()
	if err != nil {
		return append(out, check{"config", fail, err.Error()})
	}

	out = append(out, providerCheck(cfg))

	if cfg.APIKey == "" {
		out = append(out, check{"api key", fail, config.MissingKeyMessage(cfg.Provider)})
	} else {
		out = append(out, check{"api key", pass, "set (value hidden)"})
	}

	return append(out, check{"model", pass, cfg.Model})
}

func sessionCheck() check {
	dir, err := session.Dir()
	if err != nil {
		return check{"sessions", warn, err.Error()}
	}

	all, err := session.List()
	if err != nil {
		return check{"sessions", warn, err.Error()}
	}
	return check{"sessions", pass, fmt.Sprintf("%d saved in %s", len(all), dir)}
}

func workingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown: " + err.Error()
	}
	return dir
}

func binary(name, why string) check {
	path, err := exec.LookPath(name)
	if err != nil {
		return check{name, warn, "not found — " + why}
	}
	return check{name, pass, path}
}

func providerCheck(cfg config.Config) check {
	if _, err := provider.New(cfg); err != nil {
		return check{"provider", fail, err.Error()}
	}
	return check{"provider", pass, cfg.Provider}
}

func approvalsCheck() check {
	saved, err := approval.Load()
	if err != nil {
		return check{"approvals", warn, err.Error()}
	}
	if len(saved.Rules) == 0 {
		return check{"approvals", pass, "none saved"}
	}
	return check{"approvals", pass, fmt.Sprintf("%d saved", len(saved.Rules))}
}
