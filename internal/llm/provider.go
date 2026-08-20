package llm

import "context"

type ToolDef struct {
	Name        string
	Description string
	Schema      map[string]any
}

type Usage struct {
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	ReasoningTokens int
}

func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CachedTokens += other.CachedTokens
	u.ReasoningTokens += other.ReasoningTokens
}

type Request struct {
	System   string
	Messages []Message
	Tools    []ToolDef
}

type Response struct {
	Message Message
	Usage   Usage
}

type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request, onText func(string)) (*Response, error)
}
