package openai

import (
	"encoding/json"
	"testing"

	"github.com/ritik6559/cinch/internal/llm"
)

func TestReasoningRoundTripsVerbatim(t *testing.T) {
	const reasoning = `{"type":"reasoning","id":"rs_123","encrypted_content":"OPAQUE","summary":[]}`

	raw := []byte(`{"output":[` + reasoning + `,
		{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"main.go\"}"}
	],"usage":{}}`)

	resp, err := raiseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}

	items, err := lowerMessage(resp.Message)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if string(items[0]) != reasoning {
		t.Errorf("reasoning item was rewritten:\n got %s\nwant %s", items[0], reasoning)
	}

	var call struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(items[1], &call); err != nil {
		t.Fatal(err)
	}
	if call.Type != "function_call" || call.CallID != "call_1" {
		t.Errorf("unexpected call item: %+v", call)
	}
	if call.Arguments != `{"path":"main.go"}` {
		t.Errorf("arguments mangled: %s", call.Arguments)
	}
}

func TestRaiseUsage(t *testing.T) {
	raw := []byte(`{"output":[],"usage":{
		"input_tokens":1240,
		"input_tokens_details":{"cached_tokens":980},
		"output_tokens":310,
		"output_tokens_details":{"reasoning_tokens":256}
	}}`)

	resp, err := raiseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}

	want := llm.Usage{InputTokens: 1240, OutputTokens: 310, CachedTokens: 980, ReasoningTokens: 256}
	if resp.Usage != want {
		t.Errorf("got %+v, want %+v", resp.Usage, want)
	}
}

func TestLowerToolResultMarksErrors(t *testing.T) {
	msg := llm.Message{Role: llm.RoleUser, Blocks: []llm.Block{
		llm.ToolResult{ToolUseID: "call_9", Content: "file not found", IsError: true},
	}}

	items, err := lowerMessage(msg)
	if err != nil {
		t.Fatal(err)
	}

	var got struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Output string `json:"output"`
	}
	if err := json.Unmarshal(items[0], &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "function_call_output" || got.CallID != "call_9" {
		t.Errorf("unexpected item: %+v", got)
	}
	if got.Output != "Error: file not found" {
		t.Errorf("got %q, want the error marked in the text", got.Output)
	}
}

func TestLowerTextUsesRoleSpecificContentType(t *testing.T) {
	cases := []struct {
		role llm.Role
		want string
	}{
		{llm.RoleUser, "input_text"},
		{llm.RoleAssistant, "output_text"},
	}

	for _, tc := range cases {
		items, err := lowerMessage(llm.Message{
			Role:   tc.role,
			Blocks: []llm.Block{llm.Text{Text: "hello"}},
		})
		if err != nil {
			t.Fatal(err)
		}

		var got struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(items[0], &got); err != nil {
			t.Fatal(err)
		}
		if got.Role != string(tc.role) {
			t.Errorf("got role %q, want %q", got.Role, tc.role)
		}
		if len(got.Content) != 1 || got.Content[0].Type != tc.want {
			t.Errorf("role %s: got content %+v, want type %q", tc.role, got.Content, tc.want)
		}
	}
}

func TestRaiseMessageWithSeveralParts(t *testing.T) {
	raw := []byte(`{"output":[{"type":"message","content":[
		{"type":"output_text","text":"one "},
		{"type":"output_text","text":"two"}
	]}],"usage":{}}`)

	resp, err := raiseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Message.TextContent(); got != "one two" {
		t.Errorf("got %q, want %q", got, "one two")
	}
}

func TestRaiseSkipsUnknownItems(t *testing.T) {
	raw := []byte(`{"output":[
		{"type":"something_new_in_2027","payload":{}},
		{"type":"message","content":[{"type":"output_text","text":"ok"}]}
	],"usage":{}}`)

	resp, err := raiseResponse(raw)
	if err != nil {
		t.Fatalf("an unknown item must not fail the turn: %v", err)
	}
	if got := resp.Message.TextContent(); got != "ok" {
		t.Errorf("got %q, want %q", got, "ok")
	}
}

func TestRequestModelOverridesTheClientDefault(t *testing.T) {
	c := New("k", "client-default")

	payload, err := c.lowerRequest(llm.Request{Model: "from-request"})
	if err != nil {
		t.Fatal(err)
	}
	if got := payload["model"]; got != "from-request" {
		t.Errorf("got model %v, want the request's", got)
	}

	payload, err = c.lowerRequest(llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got := payload["model"]; got != "client-default" {
		t.Errorf("got model %v, want the client's default", got)
	}
}

func TestEffortIsOmittedUnlessSet(t *testing.T) {
	c := New("k", "m")

	payload, err := c.lowerRequest(llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := payload["reasoning"]; present {
		t.Error("reasoning must not be sent when no effort was asked for")
	}

	payload, err = c.lowerRequest(llm.Request{Effort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("got %T, want a reasoning object", payload["reasoning"])
	}
	if reasoning["effort"] != "high" {
		t.Errorf("got %v, want high", reasoning["effort"])
	}
}
