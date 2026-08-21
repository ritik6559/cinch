package llm

import (
	"encoding/json"
	"fmt"
)

type blockJSON struct {
	Type string `json:"type"`

	// Text
	Text string `json:"text,omitempty"`

	// Thinking and ToolUse
	ID string `json:"id,omitempty"`

	// Thinking
	Opaque json.RawMessage `json:"opaque,omitempty"`

	// ToolUse
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// ToolResult
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type messageJSON struct {
	Role   Role        `json:"role"`
	Blocks []blockJSON `json:"blocks"`
}

func (m Message) MarshalJSO() ([]byte, error) {
	out := messageJSON{
		Role:   m.Role,
		Blocks: make([]blockJSON, 0, len(m.Blocks)),
	}

	for _, block := range m.Blocks {
		switch b := block.(type) {
		case Text:
			out.Blocks = append(out.Blocks, blockJSON{
				Type: b.BlockType(),
				Text: b.Text,
			})
		case Thinking:
			out.Blocks = append(out.Blocks, blockJSON{
				Type:   b.BlockType(),
				ID:     b.ID,
				Opaque: b.Opaque,
			})
		case ToolUse:
			out.Blocks = append(out.Blocks, blockJSON{
				Type:  b.BlockType(),
				ID:    b.ID,
				Name:  b.Name,
				Input: b.Input,
			})

		case ToolResult:
			out.Blocks = append(out.Blocks, blockJSON{
				Type:      b.BlockType(),
				ToolUseID: b.ToolUseID,
				Content:   b.Content,
				IsError:   b.IsError,
			})
		default:
			return nil, fmt.Errorf("llm: cannot save block type %q", block.BlockType())
		}
	}
	return json.Marshal(out)
}

func (m *Message) UnmarshalJSON(data []byte) error {
	var in messageJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}

	m.Role = in.Role
	m.Blocks = make([]Block, 0, len(in.Blocks))

	for i, b := range in.Blocks {
		switch b.Type {
		case "text":
			m.Blocks = append(m.Blocks, Text{Text: b.Text})

		case "thinking":
			m.Blocks = append(m.Blocks, Thinking{ID: b.ID, Opaque: b.Opaque})

		case "tool_use":
			m.Blocks = append(m.Blocks, ToolUse{ID: b.ID, Name: b.Name, Input: b.Input})

		case "tool_result":
			m.Blocks = append(m.Blocks, ToolResult{
				ToolUseID: b.ToolUseID,
				Content:   b.Content,
				IsError:   b.IsError,
			})

		default:
			return fmt.Errorf("llm: unknown block type %q at index %d (saved by a newer version?)", b.Type, i)
		}
	}
	return nil
}