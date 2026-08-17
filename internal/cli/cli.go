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

// Exit codes given to the shell when the program ends.
const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitInterrupted = 130
)

// ErrUsage marks a wrong command line. It ends the program with code 2.
var ErrUsage = errors.New("usage")

type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Debug  bool
}

// Command is one subcommand, described as data.
type Command struct {
	Name string
	Summary string
	Usage string
	Run func(ctx context.Context, env *Env, args []string) error
}

func commands() []*Command {
	return []*Command{
		chatCmd(),
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

// Run reads the command line and calls the right command.
//
// Global flags must come before the command name. This is a limit of the
// standard flag package: it stops reading flags at the first plain argument.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("cinch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		showVersion bool
		debug bool
		cwd string
	)

	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.BoolVar(&showVersion, "v", false, "print version and exit")
	fs.BoolVar(&debug, "debug", false, "print extra information")
	fs.StringVar(&cwd, "cwd", "", "run as if cinch started in this directory")

	if err := fs.Parse(args); err != nil {
		// -h and --help arrive here as flag.ErrHelp. That is not an error.
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return nil
		}
		return fmt.Errorf("%w: %v", ErrUsage, err)
	}

	env := &Env{Stdin: stdin, Stdout: stdout, Stderr: stderr, Debug: debug}

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
		return runChat(ctx, env, nil) // plain `cinch` opens a chat session
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
	message := fmt.Sprintf("unknown command %q", name)
	for _, c := range commands() {
		if strings.HasPrefix(c.Name, name) {
			message += fmt.Sprintf(" (did you mean %q?)", c.Name)
			break
		}
	}
	return fmt.Errorf("%w: %s", ErrUsage, message)
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

	// tabwriter lines up the second column, whatever the command name length.
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, c := range commands() {
		fmt.Fprintf(tw, "  %s\t%s\n", c.Name, c.Summary)
	}
	tw.Flush()

	fmt.Fprint(w, `
Flags:
  -v, --version   print version and exit
      --debug     print extra information
      --cwd dir   run as if cinch started in this directory
  -h, --help      show this help

Flags must come before the command name.

Environment:
  OPENAI_API_KEY   required
  OPENAI_MODEL     model name, defaults to gpt-5.6
`)
}