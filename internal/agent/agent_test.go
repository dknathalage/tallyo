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
func newTestAgent(t *testing.T, client llm.Client) (*Agent, *Store, *service.InvoiceService, context.Context) {
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
	reg.Register(NewCreateInvoiceTool(inv, cp))

	events := NewEvents()
	cfg := Config{APIKey: "test", MaxIterations: 5}.WithDefaults()
	ag := NewAgent(cfg, client, store, reg, cp, events)
	return ag, store, inv, ctx
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

// assertBalanced walks a request's message window and fails if any assistant
// tool_use block (by ToolUseID) lacks a matching user ToolResult with the same
// ToolUseID in a later message. This guards BUG 1: an unanswered tool_use makes
// the real Anthropic API reject the request.
func assertBalanced(t *testing.T, msgs []llm.Message) {
	t.Helper()
	answered := map[string]bool{}
	// Collect all tool_result ids first (they appear in later user messages).
	for i := range msgs {
		for _, tr := range msgs[i].ToolResults {
			answered[tr.ToolUseID] = true
		}
	}
	for i := range msgs {
		if msgs[i].Role != llm.RoleAssistant {
			continue
		}
		for _, b := range msgs[i].Content {
			if b.Type != llm.BlockToolUse {
				continue
			}
			if b.ToolUseID == "" {
				t.Fatalf("assistant tool_use %q has empty ToolUseID", b.ToolName)
			}
			if !answered[b.ToolUseID] {
				t.Fatalf("unbalanced window: assistant tool_use id %q (%s) has no matching tool_result", b.ToolUseID, b.ToolName)
			}
		}
	}
}

// TestExecuteFirstRequestBalanced guards BUG 1: after the plan turn persists the
// forced propose_plan tool_use, the FIRST execute-loop request must answer it
// with a tool_result so the window the real API sees is balanced.
func TestExecuteFirstRequestBalanced(t *testing.T) {
	fake := llm.NewFake(
		// 1: plan turn — forced propose_plan tool_use
		toolUseResp(llm.StopToolUse, "propose_plan", "tu_plan",
			`{"steps":[{"tool":"list_invoices","summary":"list","risk":"read"}]}`),
		// 2: execute turn 1 — end the turn immediately so the loop stops cleanly
		llm.Response{StopReason: llm.StopEndTurn, Content: []llm.Block{{Type: llm.BlockText, Text: "done"}}},
	)
	ag, store, _, ctx := newTestAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "Balance test")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := ag.Start(ctx, conv.ID, "list my invoices"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Request[0] is the forced plan call; Request[1] is the first execute call —
	// the one that must already include the answering tool_result.
	if len(fake.Requests) < 2 {
		t.Fatalf("expected >=2 requests, got %d", len(fake.Requests))
	}
	assertBalanced(t, fake.Requests[1].Messages)

	// And specifically that the propose_plan tool_use was answered.
	var answered bool
	for _, m := range fake.Requests[1].Messages {
		for _, tr := range m.ToolResults {
			if tr.ToolUseID == "tu_plan" {
				answered = true
			}
		}
	}
	if !answered {
		t.Fatalf("propose_plan tool_use tu_plan was not answered by a tool_result; messages=%+v", fake.Requests[1].Messages)
	}
}

// TestPlanRiskRegistryDerived guards BUG 2: the persisted agent_step.risk is
// derived from the registered tool, NOT the model's free-text claim. A read tool
// the model mislabels "low" persists as "read"; a risky tool the model mislabels
// "read" persists as "risky".
func TestPlanRiskRegistryDerived(t *testing.T) {
	fake := llm.NewFake(
		toolUseResp(llm.StopToolUse, "propose_plan", "tu_plan",
			`{"steps":[`+
				`{"tool":"list_invoices","summary":"list","risk":"low"},`+
				`{"tool":"create_invoice","summary":"make","risk":"read"}`+
				`]}`),
	)
	ag, store, _, ctx := newTestAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "Risk test")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	userMsg, err := store.CreateMessage(ctx, conv.ID, "user", []llm.Block{{Type: llm.BlockText, Text: "go"}}, "{}")
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	_, planMsgID, err := ag.plan(ctx, conv.ID, userMsg.ID)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	persisted, err := store.ListSteps(ctx, planMsgID)
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(persisted) != 2 {
		t.Fatalf("expected 2 persisted steps, got %d", len(persisted))
	}
	byTool := map[string]string{}
	for i := range persisted {
		byTool[persisted[i].ToolName] = persisted[i].Risk
	}
	if byTool["list_invoices"] != "read" {
		t.Fatalf("list_invoices risk = %q, want read (registry-derived, model claimed \"low\")", byTool["list_invoices"])
	}
	if byTool["create_invoice"] != "risky" {
		t.Fatalf("create_invoice risk = %q, want risky (registry-derived, model claimed \"read\")", byTool["create_invoice"])
	}
}

func TestPlanPhase(t *testing.T) {
	fake := llm.NewFake(
		toolUseResp(llm.StopToolUse, "propose_plan", "tu_plan",
			`{"steps":[{"tool":"list_invoices","summary":"list them","risk":"read"}]}`),
	)
	ag, store, _, ctx := newTestAgent(t, fake)

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
	ag, store, _, ctx := newTestAgent(t, fake)

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
	ag, store, _, ctx := newTestAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "history")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := ag.feedToolResult(ctx, conv.ID, "tu_abc", Result{JSON: []string{"INV-1"}, Render: "table"}, false); err != nil {
		t.Fatalf("feedToolResult: %v", err)
	}
	if err := ag.feedToolError(ctx, conv.ID, "tu_err", "boom"); err != nil {
		t.Fatalf("feedToolError: %v", err)
	}

	hist, err := ag.loadHistory(ctx, conv.ID)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
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

// TestExecuteRisky proves the suspend path: a risky tool_use stops the loop,
// persists an awaiting step (with pending_input + checkpoint_id + tool_use_id),
// publishes an access_request event, and does NOT create the invoice.
func TestExecuteRisky(t *testing.T) {
	const riskyInput = `{"participantId":1,"items":[{"description":"x","quantity":1,"unitPrice":10}]}`
	fake := llm.NewFake(
		// 1: plan turn
		toolUseResp(llm.StopToolUse, "propose_plan", "tu_plan",
			`{"steps":[{"tool":"create_invoice","summary":"make one","risk":"risky"}]}`),
		// 2: execute turn 1 — a risky tool use (must suspend)
		toolUseResp(llm.StopToolUse, "create_invoice", "tu_risky", riskyInput),
	)
	ag, store, inv, ctx := newTestAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "Risky test")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

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

	if err := ag.Start(ctx, conv.ID, "create an invoice"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	unsub()
	<-done

	// access_request was published; message_final was not.
	var sawAccessReq, sawFinal bool
	for _, ev := range got {
		switch ev.Type {
		case "access_request":
			sawAccessReq = true
		case "message_final":
			sawFinal = true
		}
	}
	if !sawAccessReq {
		t.Fatalf("expected an access_request event; got %+v", got)
	}
	if sawFinal {
		t.Fatalf("did not expect message_final on the suspend path")
	}

	// The loop suspended: only the plan call + one execute call ran.
	if len(fake.Requests) != 2 {
		t.Fatalf("expected 2 model calls (plan + 1 execute), got %d", len(fake.Requests))
	}

	// An awaiting step exists with the expected fields.
	steps, err := store.ListSteps(ctx, planMessageID(t, ctx, store, conv.ID))
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	var awaiting *struct {
		ToolUseID    string
		PendingInput string
		HasCP        bool
	}
	for i := range steps {
		if steps[i].Status == "awaiting" {
			awaiting = &struct {
				ToolUseID    string
				PendingInput string
				HasCP        bool
			}{steps[i].ToolUseID, steps[i].PendingInput, steps[i].CheckpointID.Valid}
			break
		}
	}
	if awaiting == nil {
		t.Fatalf("no awaiting step persisted; steps=%+v", steps)
	}
	if awaiting.ToolUseID != "tu_risky" {
		t.Fatalf("awaiting tool_use_id = %q, want tu_risky", awaiting.ToolUseID)
	}
	if awaiting.PendingInput != riskyInput {
		t.Fatalf("awaiting pending_input = %q, want %q", awaiting.PendingInput, riskyInput)
	}
	if !awaiting.HasCP {
		t.Fatalf("awaiting step missing checkpoint_id")
	}

	// No invoice was created.
	rows, err := inv.List(ctx)
	if err != nil {
		t.Fatalf("inv.List: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 invoices (risky tool suspended), got %d", len(rows))
	}
}

// planMessageID returns the id of the assistant plan message (the FIRST
// assistant message in the conversation — the turn's checkpoint/steps anchor).
func planMessageID(t *testing.T, ctx context.Context, store *Store, convID int64) int64 {
	t.Helper()
	msgs, err := store.ListMessages(ctx, convID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for i := range msgs {
		if msgs[i].Role == "assistant" {
			return msgs[i].ID
		}
	}
	t.Fatalf("no assistant message found")
	return 0
}
