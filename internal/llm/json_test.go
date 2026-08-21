package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMessageRoundTripsAllBlockKinds(t *testing.T) {
	original := []Message{
		UserText("read the config"),
		{Role: RoleAssistant, Blocks: []Block{
			Thinking{ID: "rs_1", Opaque: json.RawMessage(`{"type":"reasoning","encrypted_content":"ABC123"}`)},
			Text{Text: "I will look at it."},
			ToolUse{ID: "call_1", Name: "read_file", Input: json.RawMessage(`{"path":"cinch.yaml"}`)},
		}},
		{Role: RoleUser, Blocks: []Block{
			ToolResult{ToolUseID: "call_1", Content: "model: gpt-5.6"},
			ToolResult{ToolUseID: "call_2", Content: "no such file", IsError: true},
		}},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var loaded []Message
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	if len(loaded) != len(original) {
		t.Fatalf("got %d messages, want %d", len(loaded), len(original))
	}
	for i := range original {
		if loaded[i].Role != original[i].Role {
			t.Errorf("message %d: role %q, want %q", i, loaded[i].Role, original[i].Role)
		}
		if len(loaded[i].Blocks) != len(original[i].Blocks) {
			t.Fatalf("message %d: got %d blocks, want %d",
				i, len(loaded[i].Blocks), len(original[i].Blocks))
		}
		for j := range original[i].Blocks {
			if !reflect.DeepEqual(loaded[i].Blocks[j], original[i].Blocks[j]) {
				t.Errorf("message %d block %d:\n got %#v\nwant %#v",
					i, j, loaded[i].Blocks[j], original[i].Blocks[j])
			}
		}
	}
}

// The most important property in this file.
//
// Thinking.Opaque holds the model's encrypted reasoning. If it does not survive
// being saved and loaded, resuming a session silently produces a worse agent,
// with no error to tell you why.
//
// Note the comparison: json.Marshal compacts raw JSON, so whitespace between
// tokens may change. What must not change is anything inside — above all the
// encrypted_content value.
func TestThinkingOpaqueSurvives(t *testing.T) {
	const reasoning = `{"type": "reasoning", "id": "rs_1", "encrypted_content": "SECRET==", "summary": []}`

	data, err := json.Marshal(Message{
		Role:   RoleAssistant,
		Blocks: []Block{Thinking{ID: "rs_1", Opaque: json.RawMessage(reasoning)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var loaded Message
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	thinking, ok := loaded.Blocks[0].(Thinking)
	if !ok {
		t.Fatalf("got %T, want Thinking", loaded.Blocks[0])
	}

	var want bytes.Buffer
	if err := json.Compact(&want, []byte(reasoning)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(thinking.Opaque, want.Bytes()) {
		t.Errorf("reasoning changed:\n got %s\nwant %s", thinking.Opaque, want.Bytes())
	}

	// The part that must be exact.
	if !strings.Contains(string(thinking.Opaque), `"encrypted_content":"SECRET=="`) {
		t.Errorf("the encrypted content did not survive: %s", thinking.Opaque)
	}
}

// A file written by a newer cinch must fail clearly, not lose blocks quietly.
func TestUnknownBlockTypeIsAnError(t *testing.T) {
	data := []byte(`{"role":"assistant","blocks":[{"type":"image","url":"x"}]}`)

	var m Message
	err := json.Unmarshal(data, &m)
	if err == nil {
		t.Fatal("expected an error for an unknown block type")
	}
	if !strings.Contains(err.Error(), "image") {
		t.Errorf("error should name the unknown type, got: %v", err)
	}
}

func TestEmptyMessageRoundTrips(t *testing.T) {
	data, err := json.Marshal(Message{Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}

	var loaded Message
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Role != RoleUser {
		t.Errorf("got role %q, want %q", loaded.Role, RoleUser)
	}
	if len(loaded.Blocks) != 0 {
		t.Errorf("got %d blocks, want 0", len(loaded.Blocks))
	}
}

// Every block kind must be writable. A new kind added later without a case in
// MarshalJSON would otherwise be discovered only when a session failed to save.
func TestEveryBlockKindCanBeSaved(t *testing.T) {
	kinds := []Block{
		Text{Text: "x"},
		Thinking{ID: "r", Opaque: json.RawMessage(`{}`)},
		ToolUse{ID: "c", Name: "n", Input: json.RawMessage(`{}`)},
		ToolResult{ToolUseID: "c", Content: "y"},
	}

	for _, block := range kinds {
		if _, err := json.Marshal(Message{Role: RoleUser, Blocks: []Block{block}}); err != nil {
			t.Errorf("%s cannot be saved: %v", block.BlockType(), err)
		}
	}
}