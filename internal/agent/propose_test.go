package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

type proposeProbe struct {
	Code     string `json:"code"`
	Quantity int    `json:"quantity"`
}

func TestProposeDecodesForcedToolUse(t *testing.T) {
	fake := &llm.Fake{}
	// SetResponses is variadic by VALUE: func (f *Fake) SetResponses(rs ...Response).
	fake.SetResponses(llm.Response{
		StopReason: llm.StopToolUse,
		Content: []llm.Block{{
			Type:     llm.BlockToolUse,
			ToolName: "emit",
			Input:    json.RawMessage(`{"code":"01_011_0107_1_1","quantity":3}`),
		}},
	})
	got, err := propose[proposeProbe](context.Background(), fake, Config{Model: "claude-x"},
		"system", "user", "emit", json.RawMessage(`{"type":"object"}`))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if got.Code != "01_011_0107_1_1" || got.Quantity != 3 {
		t.Fatalf("decoded = %+v", got)
	}
}

func TestProposeErrorsWhenNoToolCall(t *testing.T) {
	fake := &llm.Fake{}
	fake.SetResponses(llm.Response{
		StopReason: llm.StopEndTurn,
		Content:    []llm.Block{{Type: llm.BlockText, Text: "I refuse"}},
	})
	_, err := propose[proposeProbe](context.Background(), fake, Config{Model: "claude-x"},
		"system", "user", "emit", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error when model emits no tool call")
	}
}
