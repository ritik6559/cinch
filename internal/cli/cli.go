package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/ritik6559/cinch/internal/version"
)

const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitInterrupted = 130
)

var ErrUsage = errors.New("usage")

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

func (e *usageError) Is(target error) bool { return target == ErrUsage }

func usagef(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

type Env struct {
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Debug    bool
	Continue bool
	Resume   string
	NoTUI    bool
}

type Command struct {
	Name    string
	Summary string
	Usage   string
	Run     func(ctx context.Context, env *Env, args []string) error
}

func commands() []*Command {
	return []*Command{
		approvalsCmd(),
		chatCmd(),
		doctorCmd(),
		sessionsCmd(),
		skillsCmd(),
		versionCmd(),
	}
}

func lookup(name string) *Command {
	for _, c := range commands() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("cinch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		showVersion bool
		debug       bool
		cwd         string
		resume      string
		cont        bool
		noTUI       bool
	)

	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.BoolVar(&showVersion, "v", false, "print version and exit")
	fs.BoolVar(&debug, "debug", false, "print extra information")
	fs.StringVar(&cwd, "cwd", "", "run as if cinch started in this directory")
	fs.StringVar(&resume, "resume", "", "resume a saved session by id")
	fs.BoolVar(&cont, "continue", false, "resume the most recent session")
	fs.BoolVar(&cont, "c", false, "resume the most recent session")
	fs.BoolVar(&noTUI, "no-tui", false, "use the plain prompt instead of the full interface")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return nil
		}
		return usagef("%v", err)
	}

	env := &Env{
		Stdin:    stdin,
		Stdout:   stdout,
		Stderr:   stderr,
		Debug:    debug,
		Continue: cont,
		Resume:   resume,
		NoTUI:    noTUI,
	}

	if cwd != "" {
		if err := os.Chdir(cwd); err != nil {
			return fmt.Errorf("cwd: %w", err)
		}
	}

	if showVersion {
		fmt.Fprintln(stdout, version.Detailed())
		return nil
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return runChat(ctx, env, nil)
	}

	name, commandArgs := rest[0], rest[1:]

	if name == "help" {
		if len(commandArgs) == 0 {
			printUsage(stdout)
			return nil
		}
		c := lookup(commandArgs[0])
		if c == nil {
			return unknown(commandArgs[0])
		}
		fmt.Fprintf(stdout, "%s\n\n%s\n", c.Summary, c.Usage)
		return nil
	}

	c := lookup(name)
	if c == nil {
		return unknown(name)
	}
	return c.Run(ctx, env, commandArgs)
}

func unknown(name string) error {
	if match := suggest(name); match != "" {
		return usagef("unknown command %q (did you mean %q?)", name, match)
	}
	return usagef("unknown command %q. Run 'cinch help' to see the commands", name)
}

func suggest(name string) string {
	best, bestDistance := "", 3

	for _, c := range commands() {
		if strings.HasPrefix(c.Name, name) {
			return c.Name
		}
		if d := editDistance(name, c.Name); d < bestDistance {
			best, bestDistance = c.Name, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)

	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			replace := 1
			if a[i-1] == b[j-1] {
				replace = 0
			}
			current[j] = min(
				previous[j]+1,
				current[j-1]+1,
				previous[j-1]+replace,
			)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func ExitCode(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, context.Canceled):
		return ExitInterrupted
	case errors.Is(err, ErrUsage):
		return ExitUsage
	default:
		return ExitError
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, "cinch %s — a coding agent\n\n", version.Short())
	fmt.Fprint(w, "Usage:\n  cinch [flags] [command]\n\nCommands:\n")

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, c := range commands() {
		fmt.Fprintf(tw, "  %s\t%s\n", c.Name, c.Summary)
	}
	tw.Flush()

	fmt.Fprint(w, `
Flags:
  -c, --continue   resume the most recent session
      --resume id  resume a saved session by id
  -v, --version    print version and exit
      --debug      print extra information
      --cwd dir    run as if cinch started in this directory
      --no-tui     use the plain prompt instead of the full interface
  -h, --help       show this help

Flags must come before the command name.

Environment:
  OPENAI_API_KEY   required
  OPENAI_MODEL     model name, defaults to gpt-5.6
`)
}
