package llm

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// (a) + the StopReason for a tool_use response maps through fromSDK.
//
// The SDK's ContentBlockUnion.As*() accessors decode from an internal raw-JSON
// field that is only populated by real deserialization, so we build the response
// by unmarshaling a wire-shaped JSON document rather than a Go struct literal.
func TestFromSDK_ToolUseBlock(t *testing.T) {
	const wire = `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-opus-4-8",
		"stop_reason": "tool_use",
		"content": [
			{"type": "text", "text": "thinking out loud"},
			{"type": "tool_use", "id": "toolu_1", "name": "create_invoice", "input": {"amount": 42}}
		],
		"usage": {
			"input_tokens": 10,
			"output_tokens": 20,
			"cache_read_input_tokens": 3,
			"cache_creation_input_tokens": 4
		}
	}`
	var msg anthropic.Message
	if err := json.Unmarshal([]byte(wire), &msg); err != nil {
		t.Fatalf("unmarshal wire message: %v", err)
	}

	got := fromSDK(&msg)
	if got.StopReason != StopToolUse {
		t.Fatalf("StopReason = %q, want %q", got.StopReason, StopToolUse)
	}
	if len(got.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2", len(got.Content))
	}
	if got.Content[0].Type != BlockText || got.Content[0].Text != "thinking out loud" {
		t.Fatalf("Content[0] = %+v, want text block", got.Content[0])
	}
	tu := got.Content[1]
	if tu.Type != BlockToolUse {
		t.Fatalf("Content[1].Type = %q, want %q", tu.Type, BlockToolUse)
	}
	if tu.ToolUseID != "toolu_1" {
		t.Fatalf("ToolUseID = %q, want toolu_1", tu.ToolUseID)
	}
	if tu.ToolName != "create_invoice" {
		t.Fatalf("ToolName = %q, want create_invoice", tu.ToolName)
	}
	var gotInput map[string]int
	if err := json.Unmarshal(tu.Input, &gotInput); err != nil {
		t.Fatalf("tool input not valid json: %v (%q)", err, string(tu.Input))
	}
	if gotInput["amount"] != 42 {
		t.Fatalf("tool input = %v, want amount=42", gotInput)
	}
}

// (b) refusal stop reason maps to StopRefusal.
func TestMapStopReason(t *testing.T) {
	cases := map[anthropic.StopReason]string{
		anthropic.StopReasonRefusal:   StopRefusal,
		anthropic.StopReasonToolUse:   StopToolUse,
		anthropic.StopReasonEndTurn:   StopEndTurn,
		anthropic.StopReasonMaxTokens: StopMaxTok,
	}
	for in, want := range cases {
		if got := mapStopReason(in); got != want {
			t.Errorf("mapStopReason(%q) = %q, want %q", in, got, want)
		}
	}
	// Unknown reason passes through as its raw string.
	if got := mapStopReason(anthropic.StopReasonStopSequence); got != "stop_sequence" {
		t.Errorf("mapStopReason(stop_sequence) = %q, want passthrough", got)
	}
}

func TestFromSDK_RefusalStopReason(t *testing.T) {
	msg := &anthropic.Message{StopReason: anthropic.StopReasonRefusal}
	if got := fromSDK(msg); got.StopReason != StopRefusal {
		t.Fatalf("StopReason = %q, want %q", got.StopReason, StopRefusal)
	}
}

// (c) a user message carrying a tool_result maps to one SDK user message with
// a tool_result block bearing the id and is_error=true.
func TestToSDKMessages_ToolResult(t *testing.T) {
	msgs := []Message{{
		Role:        RoleUser,
		ToolResults: []ToolResult{{ToolUseID: "t1", Content: "{}", IsError: true}},
	}}

	out := toSDKMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}

	// Round-trip through JSON to assert structurally on the produced block.
	b, err := json.Marshal(out[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		Role    string `json:"role"`
		Content []struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
			IsError   bool   `json:"is_error"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Role != "user" {
		t.Fatalf("role = %q, want user", got.Role)
	}
	if len(got.Content) != 1 {
		t.Fatalf("len(content) = %d, want 1", len(got.Content))
	}
	if got.Content[0].Type != "tool_result" {
		t.Fatalf("content type = %q, want tool_result", got.Content[0].Type)
	}
	if got.Content[0].ToolUseID != "t1" {
		t.Fatalf("tool_use_id = %q, want t1", got.Content[0].ToolUseID)
	}
	if !got.Content[0].IsError {
		t.Fatalf("is_error = false, want true")
	}
}

// Assistant text + tool_use round-trip in the request direction.
func TestToSDKMessages_AssistantBlocks(t *testing.T) {
	msgs := []Message{{
		Role: RoleAssistant,
		Content: []Block{
			{Type: BlockText, Text: "ok"},
			{Type: BlockToolUse, ToolUseID: "u1", ToolName: "list_invoices", Input: json.RawMessage(`{"status":"draft"}`)},
		},
	}}
	out := toSDKMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	b, err := json.Marshal(out[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Role != "assistant" {
		t.Fatalf("role = %q, want assistant", got.Role)
	}
	if len(got.Content) != 2 {
		t.Fatalf("len(content) = %d, want 2", len(got.Content))
	}
	if got.Content[1].Type != "tool_use" || got.Content[1].Name != "list_invoices" || got.Content[1].ID != "u1" {
		t.Fatalf("tool_use block = %+v", got.Content[1])
	}
}
