package llm

import (
	"context"
	"slices"
)

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

var Efforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

func ValidEffort(s string) bool {
	return slices.Contains(Efforts, s)
}

type Request struct {
	System   string
	Messages []Message
	Tools    []ToolDef
	Model    string
	Effort   string
}

type Response struct {
	Message Message
	Usage   Usage
}

type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request, onText func(string)) (*Response, error)
}
