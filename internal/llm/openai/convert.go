package openai

import (
	"encoding/json"
	"fmt"

	"github.com/ritik6559/cinch/internal/llm"
)

type toolDef struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

func (c *Client) lowerRequest(req llm.Request) (map[string]any, error) {
	input := make([]json.RawMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		items, err := lowerMessage(m)
		if err != nil {
			return nil, err
		}
		input = append(input, items...)
	}

	tools := make([]toolDef, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, toolDef{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Schema,
		})
	}

	payload := map[string]any{
		"model":   c.model,
		"input":   input,
		"tools":   tools,
		"store":   false,
		"stream":  true,
		"include": []string{"reasoning.encrypted_content"},
	}
	if req.System != "" {
		payload["instructions"] = req.System
	}
	return payload, nil
}

type responseBody struct {
	Output []json.RawMessage `json:"output"`
	Usage  struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		OutputTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

// raiseResponse turns the provider's answer into one canonical assistant turn.
func raiseResponse(raw []byte) (*llm.Response, error) {
	var body responseBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	message := llm.Message{Role: llm.RoleAssistant}
	for _, item := range body.Output {
		blocks, err := raiseItem(item)
		if err != nil {
			return nil, err
		}
		message.Blocks = append(message.Blocks, blocks...)
	}

	return &llm.Response{
		Message: message,
		Usage: llm.Usage{
			InputTokens:     body.Usage.InputTokens,
			OutputTokens:    body.Usage.OutputTokens,
			CachedTokens:    body.Usage.InputTokensDetails.CachedTokens,
			ReasoningTokens: body.Usage.OutputTokensDetails.ReasoningTokens,
		},
	}, nil
}

func raiseItem(item json.RawMessage) ([]llm.Block, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(item, &head); err != nil {
		return nil, fmt.Errorf("openai: malformed output item: %w", err)
	}

	switch head.Type {
	case "reasoning":
		var r struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(item, &r); err != nil {
			return nil, err
		}
		opaque := make(json.RawMessage, len(item))
		copy(opaque, item)
		return []llm.Block{llm.Thinking{ID: r.ID, Opaque: opaque}}, nil

	case "message":
		var m struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(item, &m); err != nil {
			return nil, err
		}
		var blocks []llm.Block
		for _, c := range m.Content {
			if c.Type == "output_text" {
				blocks = append(blocks, llm.Text{Text: c.Text})
			}
		}
		return blocks, nil

	case "function_call":
		var fc struct {
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := json.Unmarshal(item, &fc); err != nil {
			return nil, err
		}
		return []llm.Block{llm.ToolUse{
			ID:    fc.CallID,
			Name:  fc.Name,
			Input: json.RawMessage(fc.Arguments),
		}}, nil
	}

	return nil, nil
}

func lowerMessage(m llm.Message) ([]json.RawMessage, error) {
	items := make([]json.RawMessage, 0, len(m.Blocks))

	for _, block := range m.Blocks {
		switch b := block.(type) {
		case llm.Text:
			contentType := "input_text"
			if m.Role == llm.RoleAssistant {
				contentType = "output_text"
			}
			item, err := marshal(map[string]any{
				"role": string(m.Role),
				"content": []map[string]any{
					{"type": contentType, "text": b.Text},
				},
			})
			if err != nil {
				return nil, err
			}
			items = append(items, item)

		case llm.Thinking:
			if len(b.Opaque) > 0 {
				items = append(items, b.Opaque)
			}

		case llm.ToolUse:
			item, err := marshal(map[string]any{
				"type":      "function_call",
				"call_id":   b.ID,
				"name":      b.Name,
				"arguments": string(b.Input),
			})
			if err != nil {
				return nil, err
			}
			items = append(items, item)

		case llm.ToolResult:
			output := b.Content
			if b.IsError {
				output = "Error: " + output
			}
			item, err := marshal(map[string]any{
				"type":    "function_call_output",
				"call_id": b.ToolUseID,
				"output":  output,
			})
			if err != nil {
				return nil, err
			}
			items = append(items, item)

		default:
			return nil, fmt.Errorf("openai: cannot lower block type %q", block.BlockType())
		}
	}
	return items, nil
}

func marshal(v any) (json.RawMessage, error) {
	return json.Marshal(v)
}
