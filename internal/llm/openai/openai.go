package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiUrl = "https://api.openai.com/v1/responses"

type Client struct {
	apikey string
	model  string
	http   *http.Client
}

func New(apiKey, model string) *Client {
	return &Client{
		apikey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 5 * time.Minute},
	}
}

type Tool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type FunctionCall struct {
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Usage struct {
	InputToken   int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Response struct {
	Output []json.RawMessage `json:"output"`
	Usage  Usage             `json:"usage"`
}


func (c *Client) Call(ctx context.Context, input []json.RawMessage, tools[] Tool) (*Response, error) {
	body, err := json.Marshal(map[string]any{
		"model": c.model,
		"input": input,
		"tools": tools,
		"store": false,
		"include": []string{"reasoning.encrypted_content"},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiUrl, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apikey)
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http: %d: %s", httpResp.StatusCode, raw)
	}

	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (r *Response) Texts() []string {
	var out []string
	for _, item := range r.Output {
		if itemType(item) != "message" {
			continue
		}
		var m struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(item, &m); err != nil {
			continue
		}
		for _, c := range m.Content {
			if c.Type == "output_text" {
				out = append(out, c.Text)
			}
		}
	}

	return out
}

func (r *Response) Calls() []FunctionCall {
	var out []FunctionCall
	for _, item := range r.Output {
		if itemType(item) != "function_call" {
			continue
		}
		var fc FunctionCall
		if err := json.Unmarshal(item, &fc); err != nil {
			continue
		}
		out = append(out, fc)
	}
	return out
}

func UserMessage(text string) json.RawMessage {
	return mustJSON(map[string]any{
		"role":    "user",
		"content": text,
	})
}

func ToolResult(callID, output string) json.RawMessage {
	return mustJSON(map[string]any{
		"type":    "function_call_output",
		"call_id": callID,
		"output":  output,
	})
}

func itemType(raw json.RawMessage) string {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return ""
	}
	return head.Type
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}