package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/tools"
)

const maxSteps = 25

var ErrMaxSteps = errors.New("reached step limit")

type Approver func(tool, summary string) bool

type Agent struct {
	provider llm.Provider
	tools    *tools.Tools
	approver Approver
	system   string
	out      io.Writer
	messages []llm.Message

	usage     llm.Usage
	turnUsage llm.Usage
}

func New(provider llm.Provider, tls *tools.Tools, approver Approver, out io.Writer) *Agent {
	return &Agent{
		provider: provider,
		tools:    tls,
		approver: approver,
		system:   DefaultSystemPrompt,
		out:      out,
	}
}

func (a *Agent) SetSystemPrompt(s string) { a.system = s }

func (a *Agent) Usage() llm.Usage { return a.usage }

func (a *Agent) TurnUsage() llm.Usage { return a.turnUsage }

func (a *Agent) Messages() []llm.Message { return a.messages }

func (a *Agent) Run(ctx context.Context, prompt string) error {
	a.messages = append(a.messages, llm.UserText(prompt))
	a.turnUsage = llm.Usage{}

	for step := range maxSteps {
		printer := &streamPrinter{out: a.out}
		resp, err := a.provider.Complete(ctx, llm.Request{
			System:   a.system,
			Messages: a.messages,
			Tools:    a.tools.Definitions(),
		}, printer.write)
		printer.done()
		if err != nil {
			return err
		}

		a.messages = append(a.messages, resp.Message)
		a.turnUsage.Add(resp.Usage)
		a.usage.Add(resp.Usage)
		// The text is already on screen: printer wrote it as it arrived.

		calls := resp.Message.ToolUses()
		if len(calls) == 0 {
			return nil
		}
		if step == maxSteps-1 {
			// No budget left for the model to see the results, so don't
			// run tools with side effects we can never follow up on.
			return fmt.Errorf("%w of %d", ErrMaxSteps, maxSteps)
		}

		results := make([]llm.Block, 0, len(calls))
		for _, call := range calls {
			summary := tools.Summary(call.Name, string(call.Input))
			fmt.Fprintf(a.out, " -> %s\n", summary)
			results = append(results, a.execute(ctx, call, summary))
		}
		a.messages = append(a.messages, llm.Message{Role: llm.RoleUser, Blocks: results})
	}

	return fmt.Errorf("%w of %d", ErrMaxSteps, maxSteps)
}

func (a *Agent) execute(ctx context.Context, call llm.ToolUse, summary string) llm.ToolResult {
	if a.approver != nil && a.tools.NeedsApproval(call.Name) {
		if !a.approver(call.Name, summary) {
			return llm.ToolResult{
				ToolUseID: call.ID,
				Content: "denied: the user rejected this tool call. " +
					"Do not retry it — explain what you were going to do and ask how to proceed.",
				IsError: true,
			}
		}
	}

	return llm.ToolResult{
		ToolUseID: call.ID,
		Content:   a.tools.Run(ctx, call.Name, string(call.Input)),
	}
}

type streamPrinter struct {
	out     io.Writer
	started bool
}

func (p *streamPrinter) write(text string) {
	if !p.started {
		fmt.Fprint(p.out, "\ncinch: ")
		p.started = true
	}
	fmt.Fprint(p.out, text)
}

func (p *streamPrinter) done() {
	if p.started {
		fmt.Fprintln(p.out)
		p.started = false
	}
}
