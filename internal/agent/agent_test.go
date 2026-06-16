package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

// newTestAgent wires a real Store + InvoiceService over the same temp DB, a
// Registry with list_invoices + propose_plan, a Checkpoint, Events, and the
// supplied scripted llm. It returns the agent, store, and an authed context.
func newTestAgent(t *testing.T, client llm.Client) (*Agent, *Store, context.Context) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	tenantID, userID := seedTenantUser(t, conn)
	ctx := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)

	store := NewStore(conn)
	hub := realtime.NewHub()
	inv := service.NewInvoiceService(conn, hub)
	cp := NewCheckpoint(store, conn)

	reg := NewRegistry()
	reg.Register(NewListInvoicesTool(inv))

	events := NewEvents()
	cfg := Config{APIKey: "test", MaxIterations: 5}.WithDefaults()
	ag := NewAgent(cfg, client, store, reg, cp, events)
	return ag, store, ctx
}

func toolUseResp(stop, toolName, useID, input string) llm.Response {
	return llm.Response{
		StopReason: stop,
		Content: []llm.Block{{
			Type: llm.BlockToolUse, ToolUseID: useID, ToolName: toolName,
			Input: json.RawMessage(input),
		}},
	}
}

func TestPlanPhase(t *testing.T) {
	fake := llm.NewFake(
		toolUseResp(llm.StopToolUse, "propose_plan", "tu_plan",
			`{"steps":[{"tool":"list_invoices","summary":"list them","risk":"read"}]}`),
	)
	ag, store, ctx := newTestAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "Plan test")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	userMsg, err := store.CreateMessage(ctx, conv.ID, "user", []llm.Block{{Type: llm.BlockText, Text: "list my invoices"}}, "{}")
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	steps, planMsgID, err := ag.plan(ctx, conv.ID, userMsg.ID)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(steps) != 1 || steps[0].Tool != "list_invoices" || steps[0].Risk != "read" {
		t.Fatalf("plan steps = %+v", steps)
	}

	// call 1 forced propose_plan
	if len(fake.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(fake.Requests))
	}
	if fake.Requests[0].ToolChoice.ForceTool != "propose_plan" {
		t.Fatalf("call 1 ForceTool = %q, want propose_plan", fake.Requests[0].ToolChoice.ForceTool)
	}

	// planned steps persisted under the plan message
	persisted, err := store.ListSteps(ctx, planMsgID)
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(persisted) != 1 || persisted[0].Status != "planned" || persisted[0].ToolName != "list_invoices" {
		t.Fatalf("persisted steps = %+v", persisted)
	}
}

func TestExecuteReads(t *testing.T) {
	fake := llm.NewFake(
		// 1: plan turn
		toolUseResp(llm.StopToolUse, "propose_plan", "tu_plan",
			`{"steps":[{"tool":"list_invoices","summary":"list","risk":"read"}]}`),
		// 2: execute turn 1 — a read tool use
		toolUseResp(llm.StopToolUse, "list_invoices", "tu_list", `{}`),
		// 3: execute turn 2 — end turn
		llm.Response{StopReason: llm.StopEndTurn, Content: []llm.Block{{Type: llm.BlockText, Text: "here are your invoices"}}},
	)
	ag, store, ctx := newTestAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "Execute test")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	// Subscribe BEFORE Start to collect events.
	ch, unsub := ag.events.Subscribe(conv.ID)
	defer unsub()
	var got []Event
	done := make(chan struct{})
	go func() {
		for ev := range ch {
			got = append(got, ev)
		}
		close(done)
	}()

	if err := ag.Start(ctx, conv.ID, "list my invoices"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	unsub()
	<-done

	// No access_request emitted (all reads).
	for _, ev := range got {
		if ev.Type == "access_request" {
			t.Fatalf("unexpected access_request event")
		}
	}
	// A tool_result and message_final were published.
	var sawToolResult, sawFinal bool
	for _, ev := range got {
		if ev.Type == "tool_result" {
			sawToolResult = true
		}
		if ev.Type == "message_final" {
			sawFinal = true
		}
	}
	if !sawToolResult {
		t.Fatalf("expected a tool_result event; got %+v", got)
	}
	if !sawFinal {
		t.Fatalf("expected a message_final event; got %+v", got)
	}

	// Final assistant message persisted.
	msgs, err := store.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	var sawFinalText bool
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.Type == llm.BlockText && b.Text == "here are your invoices" {
				sawFinalText = true
			}
		}
	}
	if !sawFinalText {
		t.Fatalf("final assistant text not persisted; messages=%+v", msgs)
	}

	// Fake call sequence: call 1 forced, calls >1 auto.
	if len(fake.Requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(fake.Requests))
	}
	if fake.Requests[0].ToolChoice.ForceTool != "propose_plan" {
		t.Fatalf("call 1 ForceTool = %q, want propose_plan", fake.Requests[0].ToolChoice.ForceTool)
	}
	for i := 1; i < len(fake.Requests); i++ {
		if fake.Requests[i].ToolChoice.ForceTool != "" {
			t.Fatalf("call %d ForceTool = %q, want empty (auto)", i+1, fake.Requests[i].ToolChoice.ForceTool)
		}
	}

	// A checkpoint was opened and committed.
	var found bool
	for id := int64(1); id <= 10 && !found; id++ {
		c, e := store.GetCheckpoint(ctx, id)
		if e != nil {
			continue
		}
		found = true
		if c.Status != "committed" {
			t.Fatalf("checkpoint %d status = %q, want committed", id, c.Status)
		}
	}
	if !found {
		t.Fatalf("no checkpoint opened")
	}
}

// TestLoadHistoryToolResultRoundTrip proves a persisted tool result reloads as
// an llm.Message the adapter turns into a tool_result block (Task 9 resume).
func TestLoadHistoryToolResultRoundTrip(t *testing.T) {
	fake := llm.NewFake()
	ag, store, ctx := newTestAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "history")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	ag.feedToolResult(ctx, conv.ID, "tu_abc", Result{JSON: []string{"INV-1"}, Render: "table"}, false)
	ag.feedToolError(ctx, conv.ID, "tu_err", "boom")

	hist := ag.loadHistory(ctx, conv.ID)
	var okResult, okErr bool
	for _, m := range hist {
		for _, tr := range m.ToolResults {
			if tr.ToolUseID == "tu_abc" && !tr.IsError {
				okResult = true
			}
			if tr.ToolUseID == "tu_err" && tr.IsError {
				okErr = true
			}
		}
	}
	if !okResult {
		t.Fatalf("tool result did not round-trip as a tool_result; hist=%+v", hist)
	}
	if !okErr {
		t.Fatalf("tool error did not round-trip as an is_error tool_result; hist=%+v", hist)
	}
}
