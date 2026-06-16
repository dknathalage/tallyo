package llm

import (
	"context"
	"testing"
)

func TestFakeScriptsTurns(t *testing.T) {
	f := NewFake(
		Response{StopReason: StopToolUse, Content: []Block{{Type: BlockToolUse, ToolUseID: "t1", ToolName: "list_invoices", Input: []byte(`{}`)}}},
		Response{StopReason: StopEndTurn, Content: []Block{{Type: BlockText, Text: "done"}}},
	)
	r1, err := f.CreateMessage(context.Background(), Request{})
	if err != nil || r1.StopReason != StopToolUse {
		t.Fatalf("turn1: %+v %v", r1, err)
	}
	r2, _ := f.CreateMessage(context.Background(), Request{})
	if r2.Content[0].Text != "done" {
		t.Fatalf("turn2: %+v", r2)
	}
	if f.Calls() != 2 {
		t.Fatalf("calls=%d", f.Calls())
	}
}
