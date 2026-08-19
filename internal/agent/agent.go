package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ritik6559/cinch/internal/llm/openai"
	"github.com/ritik6559/cinch/internal/tools"
)

const maxSteps = 25

var ErrMaxSteps = errors.New("reached step limit")

type Approver func(tool, summary string) bool

type Agent struct {
	client   *openai.Client
	tools    *tools.Tools
	approver Approver
	system   string
	out      io.Writer
	input    []json.RawMessage

	usage     openai.Usage
	turnUsage openai.Usage
}

func New(client *openai.Client, tls *tools.Tools, approver Approver, out io.Writer) *Agent {
	return &Agent{
		client:   client,
		tools:    tls,
		approver: approver,
		system:   DefaultSystemPrompt,
		out:      out,
	}
}

func (a *Agent) SetSystemPrompt(s string) { a.system = s }

func (a *Agent) Usage() openai.Usage { return a.usage }

func (a *Agent) TurnUsage() openai.Usage { return a.turnUsage }

func (a *Agent) Run(ctx context.Context, prompt string) error {
	a.input = append(a.input, openai.UserMessage(prompt))
	a.turnUsage = openai.Usage{}

	for step := range maxSteps {
		resp, err := a.client.Call(ctx, a.system, a.input, a.tools.Definitions())
		if err != nil {
			return err
		}

		a.input = append(a.input, resp.Output...)
		a.turnUsage.Add(resp.Usage)
		a.usage.Add(resp.Usage)
		for _, text := range resp.Texts() {
			fmt.Fprintf(a.out, "\ncinch: %s\n", text)
		}

		calls := resp.Calls()
		if len(calls) == 0 {
			return nil
		}
		if step == maxSteps-1 {
			// No budget left for the model to see the results, so don't
			// run tools with side effects we can never follow up on.
			return fmt.Errorf("%w of %d", ErrMaxSteps, maxSteps)
		}

		for _, call := range calls {
			summary := tools.Summary(call.Name, call.Arguments)
			fmt.Fprintf(a.out, " -> %s\n", summary)
			a.input = append(a.input, openai.ToolResult(call.CallID, a.execute(ctx, call, summary)))
		}
	}

	return fmt.Errorf("%w of %d", ErrMaxSteps, maxSteps)
}

func (a *Agent) execute(ctx context.Context, call openai.FunctionCall, summary string) string {
	if a.approver != nil && a.tools.NeedsApproval(call.Name) {
		if !a.approver(call.Name, summary) {
			return "denied: the user rejected this tool call. " +
				"Do not retry it — explain what you were going to do and ask how to proceed."
		}
	}

	return a.tools.Run(ctx, call.Name, call.Arguments)
}
