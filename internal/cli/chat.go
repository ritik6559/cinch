package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ritik6559/cinch/internal/agent"
	"github.com/ritik6559/cinch/internal/approval"
	"github.com/ritik6559/cinch/internal/compact"
	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/projectctx"
	"github.com/ritik6559/cinch/internal/provider"
	"github.com/ritik6559/cinch/internal/session"
	"github.com/ritik6559/cinch/internal/tools"
)

func chatCmd() *Command {
	return &Command{
		Name:    "chat",
		Summary: "Start an interactive session (this is the default)",
		Usage:   "cinch chat",
		Run:     runChat,
	}
}

func runChat(ctx context.Context, env *Env, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	p, err := provider.New(cfg)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}
	ts, err := tools.New(root)
	if err != nil {
		return err
	}

	sess, err := openSession(env, cfg, root)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(env.Stdin)

	saved, err := approval.Load()
	if err != nil {
		fmt.Fprintf(env.Stderr, "warning: could not read saved approvals: %v\n", err)
		saved = &approval.Store{}
	}

	approve := approver(env, scanner, saved)

	printer := &replPrinter{env: env}
	a := agent.New(p, ts, approve, printer.hooks())
	a.SetModel(cfg.Model)
	a.SetEffort(cfg.Effort)
	deps := compactDeps{agent: a, session: sess, provider: p, limit: cfg.CompactAt}
	if len(sess.Messages) > 0 {
		a.Restore(sess.Messages, sess.Usage)
	}

	instructions, err := projectctx.Load(root)
	if err != nil {
		fmt.Fprintf(env.Stderr, "warning: could not read %s: %v\n", projectctx.FileName, err)
	}
	if instructions != "" {
		a.SetSystemPrompt(projectctx.Wrap(agent.DefaultSystemPrompt, instructions))
		fmt.Fprintf(env.Stdout, "using %s from this repository\n", projectctx.FileName)
	}

	announce(env, sess, root)

	interrupt := newInterrupter(env)
	defer interrupt.stop()

	for {
		fmt.Fprint(env.Stdout, "\nyou: ")
		if !scanner.Scan() {
			return scanner.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if line == "/compact" {
			runCompaction(ctx, env, deps, a.TurnUsage().InputTokens)
			continue
		}

		turnCtx, endTurn := interrupt.beginTurn(ctx)
		runErr := a.Run(turnCtx, line)
		endTurn()
		printer.done()

		sess.SetTitle(line)
		sess.Messages = a.Messages()
		sess.Usage = a.Usage()
		if err := sess.Save(); err != nil {
			fmt.Fprintf(env.Stderr, "warning: could not save session: %v\n", err)
		}

		switch {
		case runErr == nil:
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(runErr, context.Canceled):
			fmt.Fprintln(env.Stderr, "interrupted")
		default:
			fmt.Fprintf(env.Stderr, "error: %v\n", runErr)
		}

		fmt.Fprintln(env.Stdout, usageLine(a.TurnUsage(), a.Usage()))

		if cfg.CompactAt > 0 && a.TurnUsage().InputTokens > cfg.CompactAt {
			runCompaction(ctx, env, deps, a.TurnUsage().InputTokens)
		}
	}
}

type compactDeps struct {
	agent    *agent.Agent
	session  *session.Session
	provider llm.Provider
	limit    int
}

func runCompaction(ctx context.Context, env *Env, d compactDeps, currentTokens int) {
	messages, cleared := compact.ToolResults(d.agent.Messages(), compact.DefaultOptions())

	remaining := currentTokens - cleared.EstimatedTokens()
	var summarized compact.SummaryResult

	opts := compact.DefaultSummarizeOptions()

	if d.limit > 0 && remaining > d.limit && compact.CanSummarize(messages, opts) {
		fmt.Fprintln(env.Stdout, "  clearing was not enough, summarizing...")

		next, result, err := compact.Summarize(ctx, d.provider, messages, opts)
		if err != nil {
			fmt.Fprintf(env.Stderr, "warning: could not summarize: %v\n", err)
		} else {
			messages, summarized = next, result
		}
	}

	if cleared.Cleared == 0 && summarized.Summarized == 0 {
		fmt.Fprintln(env.Stdout, "  nothing more to compact")
		return
	}

	total := d.agent.Usage()
	total.Add(summarized.Usage)
	d.agent.Restore(messages, total)

	d.session.Messages = messages
	d.session.Usage = total
	if err := d.session.Save(); err != nil {
		fmt.Fprintf(env.Stderr, "warning: could not save session: %v\n", err)
	}

	report(env, cleared, summarized)
}

func report(env *Env, cleared compact.Result, summarized compact.SummaryResult) {
	var parts []string
	if cleared.Cleared > 0 {
		parts = append(parts, fmt.Sprintf("cleared %d tool results (about %s tokens)",
			cleared.Cleared, comma(cleared.EstimatedTokens())))
	}
	if summarized.Summarized > 0 {
		parts = append(parts, fmt.Sprintf("summarized %d messages", summarized.Summarized))
	}
	fmt.Fprintf(env.Stdout, "  compacted: %s\n", strings.Join(parts, ", "))
}

type interrupter struct {
	signals chan os.Signal
	done    chan struct{}

	mu     sync.Mutex
	cancel context.CancelFunc 
}

func newInterrupter(env *Env) *interrupter {
	in := &interrupter{
		signals: make(chan os.Signal, 1),
		done:    make(chan struct{}),
	}
	signal.Notify(in.signals, os.Interrupt)

	go func() {
		for {
			select {
			case <-in.done:
				return
			case <-in.signals:
				in.mu.Lock()
				cancel := in.cancel
				in.mu.Unlock()

				if cancel != nil {
					cancel()
					continue
				}

				fmt.Fprintln(env.Stdout)
				os.Exit(ExitInterrupted)
			}
		}
	}()

	return in
}

func (in *interrupter) beginTurn(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)

	in.mu.Lock()
	in.cancel = cancel
	in.mu.Unlock()

	return ctx, func() {
		in.mu.Lock()
		in.cancel = nil
		in.mu.Unlock()
		cancel()
	}
}

func (in *interrupter) stop() {
	signal.Stop(in.signals)
	close(in.done)
}

func openSession(env *Env, cfg config.Config, root string) (*session.Session, error) {
	if env.Resume != "" {
		return session.Load(env.Resume)
	}

	if env.Continue {
		latest, err := session.Latest()
		if err != nil {
			return nil, err
		}
		if latest != nil {
			return latest, nil
		}
		fmt.Fprintln(env.Stdout, "no saved sessions yet, starting a new one")
	}

	return session.New(root, cfg.Provider, cfg.Model), nil
}

func announce(env *Env, s *session.Session, root string) {
	if len(s.Messages) == 0 {
		fmt.Fprintln(env.Stdout, "cinch — ask about the files in this directory. ctrl-c to quit.")
		return
	}

	fmt.Fprintf(env.Stdout, "cinch — resumed %s, %d turns\n", s.ID, s.Turns())
	if s.Title != "" {
		fmt.Fprintf(env.Stdout, "  %s\n", s.Title)
	}

	if filepath.Clean(s.Workspace) != filepath.Clean(root) {
		fmt.Fprintf(env.Stderr, "warning: recorded in %s, running in %s\n", s.Workspace, root)
	}
}

func usageLine(turn, session llm.Usage) string {
	var b strings.Builder

	fmt.Fprintf(&b, "  %s in", comma(turn.InputTokens))
	if turn.CachedTokens > 0 {
		fmt.Fprintf(&b, " (%s cached)", comma(turn.CachedTokens))
	}

	fmt.Fprintf(&b, " · %s out", comma(turn.OutputTokens))
	if turn.ReasoningTokens > 0 {
		fmt.Fprintf(&b, " (%s thinking)", comma(turn.ReasoningTokens))
	}

	fmt.Fprintf(&b, " · session %s in", comma(session.InputTokens))
	return b.String()
}

func comma(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}

	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func approver(env *Env, scanner *bufio.Scanner, saved *approval.Store) agent.Approver {
	session := map[string]bool{}

	return func(tool, summary, arguments string) bool {
		command := tools.CommandOf(arguments)

		if session[tool] || saved.Allows(tool, command) {
			return true
		}

		answers := "[y/N/a/s]"
		if tool == "bash" {
			answers = "[y/N/s]"
		}
		fmt.Fprintf(env.Stdout, "\nallow %s? %s ", summary, answers)

		if !scanner.Scan() {
			return false
		}

		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "y", "yes":
			return true

		case "a", "always":
			if tool == "bash" {
				fmt.Fprintln(env.Stdout, "  not offered for bash — use s to save a command prefix")
				return false
			}
			session[tool] = true
			return true

		case "s", "save":
			remember(env, saved, tool, command)
			return true

		default:
			return false
		}
	}
}

func remember(env *Env, saved *approval.Store, tool, command string) {
	prefix := ""
	if tool == "bash" {
		prefix = approval.PrefixFor(command)
	}

	if saved.Add(tool, prefix) {
		if err := saved.Save(); err != nil {
			fmt.Fprintf(env.Stderr, "warning: could not save approval: %v\n", err)
			return
		}
	}
	fmt.Fprintf(env.Stdout, "  saved: %s\n", approval.Describe(tool, prefix))
}

type replPrinter struct {
	env     *Env
	started bool
}

func (p *replPrinter) hooks() agent.Hooks {
	return agent.Hooks{
		OnText: func(text string) {
			if !p.started {
				fmt.Fprint(p.env.Stdout, "\ncinch: ")
				p.started = true
			}
			fmt.Fprint(p.env.Stdout, text)
		},

		OnToolCall: func(name, summary string) {
			p.done()
			fmt.Fprintf(p.env.Stdout, " -> %s\n", summary)
		},
	}
}

func (p *replPrinter) done() {
	if p.started {
		fmt.Fprintln(p.env.Stdout)
		p.started = false
	}
}
