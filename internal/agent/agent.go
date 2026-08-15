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

type Agent struct {
	client *openai.Client
	tools *tools.Tools
	out    io.Writer
	input  []json.RawMessage
}

func New(client *openai.Client, tls *tools.Tools, out io.Writer) *Agent {
	return &Agent{client: client, tools: tls, out: out}
}

func (a *Agent) Run(ctx context.Context, prompt string) error {
	a.input = append(a.input, openai.UserMessage(prompt))

	for step := range maxSteps {
		resp, err := a.client.Call(ctx, a.input, a.tools.Definitions())
		if err != nil {
			return err
		}

		a.input = append(a.input, resp.Output...)
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
			fmt.Fprintf(a.out, " -> %s %s", call.Name, call.Arguments)
			result := a.tools.Run(call.Name, call.Arguments)
			a.input = append(a.input, openai.ToolResult(call.CallID, result))
		}
	}

	return fmt.Errorf("%w of %d", ErrMaxSteps, maxSteps)
}
