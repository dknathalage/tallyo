package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// supportsThinking gates adaptive thinking off for haiku-tier models only.
func TestSupportsThinking(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"claude-opus-4-8", true},
		{"claude-3-5-haiku-latest", false},
		{"claude-haiku-4", false},
		{"claude-sonnet-4-5", true},
		{"", true},
	}
	for _, c := range cases {
		if got := supportsThinking(c.model); got != c.want {
			t.Errorf("supportsThinking(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}

// NewAnthropic wires model and effort onto the struct.
func TestNewAnthropic(t *testing.T) {
	a := NewAnthropic("sk-test", "claude-opus-4-8", "high")
	if a == nil {
		t.Fatal("NewAnthropic returned nil")
	}
	if a.model != "claude-opus-4-8" {
		t.Errorf("model = %q, want claude-opus-4-8", a.model)
	}
	if a.effort != "high" {
		t.Errorf("effort = %q, want high", a.effort)
	}
}

// toSDKTools rejects an empty tool name.
func TestToSDKTools_EmptyName(t *testing.T) {
	_, err := toSDKTools([]ToolDef{{Name: ""}})
	if err == nil {
		t.Fatal("expected error for empty tool name, got nil")
	}
	if !strings.Contains(err.Error(), "empty name") {
		t.Errorf("error = %v, want mention of empty name", err)
	}
}

// toSDKTools rejects malformed JSON schema.
func TestToSDKTools_BadSchema(t *testing.T) {
	_, err := toSDKTools([]ToolDef{{Name: "x", InputSchema: json.RawMessage(`{not json`)}})
	if err == nil {
		t.Fatal("expected error for bad schema, got nil")
	}
	if !strings.Contains(err.Error(), "input schema") {
		t.Errorf("error = %v, want mention of input schema", err)
	}
}

// toSDKTools returns nil for an empty list and decodes a valid schema.
func TestToSDKTools_HappyAndEmpty(t *testing.T) {
	if out, err := toSDKTools(nil); err != nil || out != nil {
		t.Fatalf("empty: out=%v err=%v", out, err)
	}

	defs := []ToolDef{{
		Name:        "create_invoice",
		Description: "make an invoice",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"amount":{"type":"number"}},"required":["amount"]}`),
	}}
	out, err := toSDKTools(defs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	tool := out[0].OfTool
	if tool == nil {
		t.Fatal("OfTool is nil")
	}
	if tool.Name != "create_invoice" {
		t.Errorf("name = %q, want create_invoice", tool.Name)
	}
	if _, ok := tool.InputSchema.Properties.(map[string]any)["amount"]; !ok {
		t.Errorf("schema properties missing amount: %+v", tool.InputSchema.Properties)
	}
	if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "amount" {
		t.Errorf("required = %v, want [amount]", tool.InputSchema.Required)
	}
}

// fromSDK maps a thinking block to BlockThinking.
func TestFromSDK_ThinkingBlock(t *testing.T) {
	const wire = `{
		"id": "msg_2",
		"type": "message",
		"role": "assistant",
		"model": "claude-opus-4-8",
		"stop_reason": "end_turn",
		"content": [
			{"type": "thinking", "thinking": "let me reason", "signature": "sig"},
			{"type": "text", "text": "answer"}
		],
		"usage": {"input_tokens": 1, "output_tokens": 2}
	}`
	var msg anthropic.Message
	if err := json.Unmarshal([]byte(wire), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := fromSDK(&msg)
	if len(got.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2", len(got.Content))
	}
	if got.Content[0].Type != BlockThinking || got.Content[0].Text != "let me reason" {
		t.Fatalf("Content[0] = %+v, want thinking block", got.Content[0])
	}
	if got.Content[1].Type != BlockText || got.Content[1].Text != "answer" {
		t.Fatalf("Content[1] = %+v, want text block", got.Content[1])
	}
}

// CreateMessage rejects a non-positive MaxTokens before any network call.
func TestCreateMessage_MaxTokensInvalid(t *testing.T) {
	a := NewAnthropic("sk-test", "claude-opus-4-8", "")
	_, err := a.CreateMessage(context.Background(), Request{MaxTokens: 0})
	if err == nil {
		t.Fatal("expected error for MaxTokens=0, got nil")
	}
	if !strings.Contains(err.Error(), "MaxTokens") {
		t.Errorf("error = %v, want mention of MaxTokens", err)
	}
}

// CreateMessage rejects a request with no model configured anywhere.
func TestCreateMessage_NoModel(t *testing.T) {
	a := NewAnthropic("sk-test", "", "")
	_, err := a.CreateMessage(context.Background(), Request{MaxTokens: 10})
	if err == nil {
		t.Fatal("expected error for missing model, got nil")
	}
	if !strings.Contains(err.Error(), "model not configured") {
		t.Errorf("error = %v, want mention of model not configured", err)
	}
}

// CreateMessage surfaces a tool-conversion error (empty tool name) before the
// network call.
func TestCreateMessage_BadTool(t *testing.T) {
	a := NewAnthropic("sk-test", "claude-opus-4-8", "")
	_, err := a.CreateMessage(context.Background(), Request{
		MaxTokens: 10,
		Tools:     []ToolDef{{Name: ""}},
	})
	if err == nil {
		t.Fatal("expected error for bad tool, got nil")
	}
	if !strings.Contains(err.Error(), "empty name") {
		t.Errorf("error = %v, want mention of empty name", err)
	}
}

// sseMessage encodes a complete Messages streaming exchange for one text block,
// matching the event sequence the SDK's Accumulate expects.
func sseMessage(text string) string {
	var b strings.Builder
	write := func(event, data string) {
		b.WriteString("event: ")
		b.WriteString(event)
		b.WriteString("\ndata: ")
		b.WriteString(data)
		b.WriteString("\n\n")
	}
	write("message_start", `{"type":"message_start","message":{"id":"msg_x","type":"message","role":"assistant","model":"claude-opus-4-8","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":0}}}`)
	write("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	delta, _ := json.Marshal(text)
	write("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+string(delta)+`}}`)
	write("content_block_stop", `{"type":"content_block_stop","index":0}`)
	write("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7}}`)
	write("message_stop", `{"type":"message_stop"}`)
	return b.String()
}

// CreateMessage streams an SSE response from a fake server and maps it back into
// a Response. The client is pointed at the test server via WithBaseURL.
func TestCreateMessage_StreamSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(sseMessage("hello from stream"))); err != nil {
			t.Errorf("write sse: %v", err)
		}
	}))
	defer srv.Close()

	a := &Anthropic{
		c:     anthropic.NewClient(option.WithAPIKey("sk-test"), option.WithBaseURL(srv.URL)),
		model: "claude-opus-4-8",
	}
	resp, err := a.CreateMessage(context.Background(), Request{
		MaxTokens: 64,
		Messages:  []Message{{Role: RoleUser, Content: []Block{{Type: BlockText, Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if resp.StopReason != StopEndTurn {
		t.Errorf("StopReason = %q, want %q", resp.StopReason, StopEndTurn)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "hello from stream" {
		t.Fatalf("Content = %+v, want single text block", resp.Content)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 7 {
		t.Errorf("Usage = %+v, want input=5 output=7", resp.Usage)
	}
}

// CreateMessage surfaces a transport/stream error from the server.
func TestCreateMessage_StreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"boom"}}`)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	a := &Anthropic{
		c:     anthropic.NewClient(option.WithAPIKey("sk-test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0)),
		model: "claude-opus-4-8",
	}
	_, err := a.CreateMessage(context.Background(), Request{
		MaxTokens: 64,
		Messages:  []Message{{Role: RoleUser, Content: []Block{{Type: BlockText, Text: "hi"}}}},
	})
	if err == nil {
		t.Fatal("expected error from failing server, got nil")
	}
	if !strings.Contains(err.Error(), "anthropic create message") {
		t.Errorf("error = %v, want wrapped create message error", err)
	}
}

// CreateMessage builds params for system + effort + tools + forced tool choice,
// and omits adaptive thinking when a tool is forced. We capture the request body
// the server receives to assert on the shaped params.
func TestCreateMessage_ForcedToolParams(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		body = b
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(sseMessage("ok"))); err != nil {
			t.Errorf("write sse: %v", err)
		}
	}))
	defer srv.Close()

	a := &Anthropic{
		c:      anthropic.NewClient(option.WithAPIKey("sk-test"), option.WithBaseURL(srv.URL)),
		model:  "claude-opus-4-8",
		effort: "high",
	}
	_, err := a.CreateMessage(context.Background(), Request{
		MaxTokens:  64,
		System:     "be terse",
		Tools:      []ToolDef{{Name: "propose_plan", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice: ToolChoice{ForceTool: "propose_plan"},
		Messages:   []Message{{Role: RoleUser, Content: []Block{{Type: BlockText, Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	var sent struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		ToolChoice struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"tool_choice"`
		Thinking map[string]any `json:"thinking"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("unmarshal request body: %v (%q)", err, string(body))
	}
	if len(sent.System) != 1 || sent.System[0].Text != "be terse" {
		t.Errorf("system = %+v, want single 'be terse' block", sent.System)
	}
	if sent.OutputConfig.Effort != "high" {
		t.Errorf("effort = %q, want high", sent.OutputConfig.Effort)
	}
	if len(sent.Tools) != 1 || sent.Tools[0].Name != "propose_plan" {
		t.Errorf("tools = %+v, want [propose_plan]", sent.Tools)
	}
	if sent.ToolChoice.Name != "propose_plan" {
		t.Errorf("tool_choice = %+v, want propose_plan", sent.ToolChoice)
	}
	// A forced tool must disable adaptive thinking.
	if sent.Thinking != nil {
		t.Errorf("thinking = %+v, want absent when tool forced", sent.Thinking)
	}
}

// CreateMessage enables adaptive thinking on a supporting model when no tool is
// forced; per-request Model and Effort override the client defaults.
func TestCreateMessage_ThinkingEnabled(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		body = b
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(sseMessage("ok"))); err != nil {
			t.Errorf("write sse: %v", err)
		}
	}))
	defer srv.Close()

	a := &Anthropic{
		c:      anthropic.NewClient(option.WithAPIKey("sk-test"), option.WithBaseURL(srv.URL)),
		model:  "claude-3-5-haiku-latest", // overridden below
		effort: "low",                     // overridden below
	}
	_, err := a.CreateMessage(context.Background(), Request{
		MaxTokens: 64,
		Model:     "claude-opus-4-8",
		Effort:    "max",
		Messages:  []Message{{Role: RoleUser, Content: []Block{{Type: BlockText, Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	var sent struct {
		Model        string `json:"model"`
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
		Thinking map[string]any `json:"thinking"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("unmarshal request body: %v (%q)", err, string(body))
	}
	if sent.Model != "claude-opus-4-8" {
		t.Errorf("model = %q, want per-request override claude-opus-4-8", sent.Model)
	}
	if sent.OutputConfig.Effort != "max" {
		t.Errorf("effort = %q, want per-request override max", sent.OutputConfig.Effort)
	}
	if sent.Thinking == nil || sent.Thinking["type"] != "adaptive" {
		t.Errorf("thinking = %+v, want adaptive config present", sent.Thinking)
	}
}

// Fake returns an error once its scripted responses are exhausted.
func TestFake_Exhausted(t *testing.T) {
	f := NewFake() // no scripted responses
	_, err := f.CreateMessage(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error when no scripted response, got nil")
	}
	if !strings.Contains(err.Error(), "no scripted response") {
		t.Errorf("error = %v, want mention of no scripted response", err)
	}
	if f.Calls() != 0 {
		t.Errorf("Calls() = %d, want 0 after exhausted call", f.Calls())
	}
}
