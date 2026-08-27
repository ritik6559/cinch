package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/sandbox"
	"github.com/ritik6559/cinch/internal/tools"
)

const maxSteps = 25

var ErrMaxSteps = errors.New("reached step limit")

type ApprovalRequest struct {
	Tool      string
	Summary   string
	Arguments string
	Reason    string
}

type Approver func(ApprovalRequest) bool

type Hooks struct {
	// OnText receives model prose as it streams, a fragment at a time.
	OnText func(text string)

	// OnToolCall fires before a tool runs, after any approval.
	OnToolCall func(name, summary string)

	// OnToolResult fires when it finishes.
	OnToolResult func(name, result string, isError bool)

	// OnUsage fires once per provider call, not once per turn.
	OnUsage func(u llm.Usage)
}

func (h Hooks) text(s string) {
	if h.OnText != nil {
		h.OnText(s)
	}
}

func (h Hooks) toolCall(name, summary string) {
	if h.OnToolCall != nil {
		h.OnToolCall(name, summary)
	}
}

func (h Hooks) toolResult(name, result string, isError bool) {
	if h.OnToolResult != nil {
		h.OnToolResult(name, result, isError)
	}
}

func (h Hooks) usage(u llm.Usage) {
	if h.OnUsage != nil {
		h.OnUsage(u)
	}
}

type Agent struct {
	provider llm.Provider
	tools    *tools.Tools
	approver Approver
	hooks    Hooks
	sandbox  sandbox.Mode
	system   string
	model    string
	effort   string
	messages []llm.Message

	usage     llm.Usage
	turnUsage llm.Usage
}

func New(provider llm.Provider, tls *tools.Tools, approver Approver, hooks Hooks) *Agent {
	return &Agent{
		provider: provider,
		tools:    tls,
		approver: approver,
		hooks:    hooks,
		sandbox:  sandbox.ModePolicy,
		system:   DefaultSystemPrompt,
	}
}

func (a *Agent) SetSystemPrompt(s string) { a.system = s }

func (a *Agent) SetSandbox(m sandbox.Mode) {
	if m != "" {
		a.sandbox = m
	}
}

func (a *Agent) Sandbox() sandbox.Mode { return a.sandbox }

func (a *Agent) SetModel(m string) { a.model = m }

func (a *Agent) Model() string { return a.model }

func (a *Agent) SetEffort(e string) { a.effort = e }

func (a *Agent) Effort() string { return a.effort }

func (a *Agent) Usage() llm.Usage { return a.usage }

func (a *Agent) TurnUsage() llm.Usage { return a.turnUsage }

func (a *Agent) Messages() []llm.Message { return a.messages }

func (a *Agent) Restore(messages []llm.Message, usage llm.Usage) {
	a.messages = messages
	a.usage = usage
}

func (a *Agent) Run(ctx context.Context, prompt string) error {
	a.answerAbandonedCalls()
	a.messages = append(a.messages, llm.UserText(prompt))
	a.turnUsage = llm.Usage{}

	for step := range maxSteps {
		resp, err := a.provider.Complete(ctx, llm.Request{
			System:   a.system,
			Messages: a.messages,
			Tools:    a.tools.Definitions(),
			Model:    a.model,
			Effort:   a.effort,
		}, a.hooks.text)
		if err != nil {
			return err
		}

		a.messages = append(a.messages, resp.Message)
		a.turnUsage.Add(resp.Usage)
		a.usage.Add(resp.Usage)
		a.hooks.usage(resp.Usage)

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
			results = append(results, a.execute(ctx, call))
		}
		a.messages = append(a.messages, llm.Message{Role: llm.RoleUser, Blocks: results})
	}

	return fmt.Errorf("%w of %d", ErrMaxSteps, maxSteps)
}

func (a *Agent) answerAbandonedCalls() {
	if len(a.messages) == 0 {
		return
	}

	calls := a.messages[len(a.messages)-1].ToolUses()
	if len(calls) == 0 {
		return
	}

	results := make([]llm.Block, 0, len(calls))
	for _, call := range calls {
		results = append(results, llm.ToolResult{
			ToolUseID: call.ID,
			Content:   "not run: the turn was interrupted",
			IsError:   true,
		})
	}
	a.messages = append(a.messages, llm.Message{Role: llm.RoleUser, Blocks: results})
}

func (a *Agent) execute(ctx context.Context, call llm.ToolUse) llm.ToolResult {
	summary := tools.Summary(call.Name, string(call.Input))
	verdict := a.judge(call)

	if verdict.Action == sandbox.Deny {
		refusal := "refused: " + verdict.Reason + ". " +
			"This was not run and asking again will not change that. " +
			"Say what you were trying to achieve and suggest a safer way."

		a.hooks.toolResult(call.Name, refusal, true)
		return llm.ToolResult{ToolUseID: call.ID, Content: refusal, IsError: true}
	}

	if verdict.Action == sandbox.Ask && a.approver != nil {
		req := ApprovalRequest{
			Tool: call.Name, Summary: summary,
			Arguments: string(call.Input), Reason: verdict.Reason,
		}
		if !a.approver(req) {
			const denied = "denied: the user rejected this tool call. " +
				"Do not retry it — explain what you were going to do and ask how to proceed."

			a.hooks.toolResult(call.Name, denied, true)
			return llm.ToolResult{ToolUseID: call.ID, Content: denied, IsError: true}
		}
	}

	a.hooks.toolCall(call.Name, summary)

	result := a.tools.Run(ctx, call.Name, string(call.Input))
	a.hooks.toolResult(call.Name, result, strings.HasPrefix(result, "error:"))

	return llm.ToolResult{ToolUseID: call.ID, Content: result}
}

func (a *Agent) judge(call llm.ToolUse) sandbox.Verdict {
	if call.Name != "bash" {
		if a.tools.NeedsApproval(call.Name) {
			return sandbox.Verdict{Action: sandbox.Ask}
		}
		return sandbox.Verdict{Action: sandbox.Allow}
	}
	return a.sandbox.Decide(tools.CommandOf(string(call.Input)))
}
