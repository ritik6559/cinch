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
	"github.com/ritik6559/cinch/internal/compact"
	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/llm"
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
	always := map[string]bool{}

	approve := func(tool, summary string) bool {
		if always[tool] {
			return true
		}
		fmt.Fprintf(env.Stdout, "\nallow %s? [y/N/a] ", summary)
		if !scanner.Scan() {
			return false
		}
		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "y", "yes":
			return true
		case "a", "always":
			always[tool] = true
			return true
		default:
			return false
		}
	}

	a := agent.New(p, ts, approve, env.Stdout)
	deps := compactDeps{agent: a, session: sess, provider: p, limit: cfg.CompactAt}
	if len(sess.Messages) > 0 {
		a.Restore(sess.Messages, sess.Usage)
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

		// Fires one turn late: nothing counts tokens locally, so the trigger is
		// the size of the request we just sent.
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

// runCompaction shrinks the conversation in two layers.
func runCompaction(ctx context.Context, env *Env, d compactDeps, currentTokens int) {
	messages, cleared := compact.ToolResults(d.agent.Messages(), compact.DefaultOptions())

	remaining := currentTokens - cleared.EstimatedTokens()
	var summarized compact.SummaryResult

	opts := compact.DefaultSummarizeOptions()

	// Checked before announcing: a conversation can be far over the limit and
	// still have nothing to summarize, because one enormous turn is not
	// history. Saying "summarizing..." and then doing nothing is worse than
	// staying quiet.
	if d.limit > 0 && remaining > d.limit && compact.CanSummarize(messages, opts) {
		fmt.Fprintln(env.Stdout, "  clearing was not enough, summarizing...")

		next, result, err := compact.Summarize(ctx, d.provider, messages, opts)
		if err != nil {
			// Clearing may still have helped, so keep what we have.
			fmt.Fprintf(env.Stderr, "warning: could not summarize: %v\n", err)
		} else {
			messages, summarized = next, result
		}
	}

	if cleared.Cleared == 0 && summarized.Summarized == 0 {
		fmt.Fprintln(env.Stdout, "  nothing more to compact")
		return
	}

	// The summarizing call costs tokens too, and hiding that would make the
	// session total wrong.
	total := d.agent.Usage()
	total.Add(summarized.Usage)
	d.agent.Restore(messages, total)

	// Persist the smaller form, or resuming would restore everything we just
	// removed.
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
	cancel context.CancelFunc // non-nil only while a turn is running
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
