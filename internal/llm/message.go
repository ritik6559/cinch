package llm

import "encoding/json"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role   Role
	Blocks []Block
}

type Block interface {
	BlockType() string
}

type Text struct {
	Text string
}

type Thinking struct {
	ID     string
	Opaque json.RawMessage
}
type ToolUse struct {
	ID    string // provider-assigned call id
	Name  string
	Input json.RawMessage
}

type ToolResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

func (Text) BlockType() string       { return "text" }
func (Thinking) BlockType() string   { return "thinking" }
func (ToolUse) BlockType() string    { return "tool_use" }
func (ToolResult) BlockType() string { return "tool_result" }

func (m Message) ToolUses() []ToolUse {
	var out []ToolUse
	for _, b := range m.Blocks {
		if tu, ok := b.(ToolUse); ok {
			out = append(out, tu)
		}
	}
	return out
}

func (m Message) TextContent() string {
	var s string
	for _, b := range m.Blocks {
		if t, ok := b.(Text); ok {
			s += t.Text
		}
	}
	return s
}

func UserText(text string) Message {
	return Message{Role: RoleUser, Blocks: []Block{Text{Text: text}}}
}
