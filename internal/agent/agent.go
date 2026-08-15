package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ritik6559/cinch/internal/llm/openai"
	"github.com/ritik6559/cinch/internal/tools"
)

type Agent struct {
	client *openai.Client
	out io.Writer
	input []json.RawMessage
}

func New(client *openai.Client, out io.Writer) *Agent {
	return &Agent{client: client, out: out}
}

func (a *Agent) Run(ctx context.Context, prompt string) error {
	a.input = append(a.input, openai.UserMessage(prompt))

	for {
		resp, err := a.client.Call(ctx, a.input, tools.Definitions())
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

		for _, call := range calls {
			fmt.Fprintf(a.out, " -> %s %s", call.Name, call.Arguments)
			result := tools.Run(call.Name, call.Arguments)
			a.input = append(a.input, openai.ToolResult(call.CallID, result))
		}
	}
}