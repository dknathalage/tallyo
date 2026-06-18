package agent

// Tests for Pillar 2 — stall recovery in the execute loop. When the plan
// declares a write (create_invoice) but the model ends a turn in prose without
// ever calling it, Execute escalates: first it nudges (a user message telling
// the model to call the tool), then on a second stall it forces the tool via
// ToolChoice.ForceTool — all bounded by maxStalls so the loop can never spin.
//
// These tests script an llm.Fake only (no real API): response[0] is always the
// forced propose_plan turn declaring a create_invoice step (so pendingWrite
// resolves to create_invoice), followed by the execute-loop turns each case
// needs.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

// newStallAgent wires the same stack as newTestAgent (Store + InvoiceService +
// Registry with list_invoices & create_invoice) but ALSO seeds a participant
// with a wide plan window and returns its id, so an approved create_invoice with
// a custom line actually persists. It returns the agent, store, invoice service,
// the seeded participant id, and an authed context.
func newStallAgent(t *testing.T, client llm.Client) (*Agent, *Store, *service.InvoiceService, int64, context.Context) {
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

	// A participant with a plan window enclosing the custom line's service date,
	// so the filling validator accepts a custom (codeless) line on approval.
	p, err := participant.NewParticipants(conn).Create(ctx, tenantID, participant.ParticipantInput{
		Name: "Stall Participant", PlanStart: "2025-07-01", PlanEnd: "2026-06-30",
	})
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}

	store := NewStore(conn)
	hub := realtime.NewHub()
	inv := service.NewInvoiceService(conn, hub)
	cp := NewCheckpoint(store, conn)

	reg := NewRegistry()
	reg.Register(NewListInvoicesTool(inv))
	reg.Register(NewCreateInvoiceTool(inv, cp))

	events := NewEvents()
	cfg := Config{APIKey: "test", MaxIterations: 8}.WithDefaults()
	ag := NewAgent(cfg, client, store, reg, cp, events)
	return ag, store, inv, p.ID, ctx
}

// stallPlanResp is the forced propose_plan turn declaring a create_invoice step,
// so plannedWriteTool resolves pendingWrite to create_invoice.
func stallPlanResp() llm.Response {
	return toolUseResp(llm.StopToolUse, "propose_plan", "tu_plan",
		`{"steps":[{"tool":"create_invoice","summary":"make one","risk":"risky"}]}`)
}

// endTurnResp is a plain prose end_turn turn (a stall: no tool call).
func endTurnResp(text string) llm.Response {
	return llm.Response{StopReason: llm.StopEndTurn, Content: []llm.Block{{Type: llm.BlockText, Text: text}}}
}

// customCreateInvoice builds a create_invoice tool_use with a single custom
// (codeless) line, which skips catalogue validation but still needs a
// participant to exist. participantID is the seeded participant.
func customCreateInvoice(useID string, participantID int64) llm.Response {
	input := `{"participantId":` + itoa(participantID) +
		`,"items":[{"description":"Support work","serviceDate":"2026-06-09","quantity":2,"unitPrice":50}]}`
	return toolUseResp(llm.StopToolUse, "create_invoice", useID, input)
}

// awaitingStep finds the single awaiting step for the conversation's plan
// message, failing if none exists.
func awaitingStep(t *testing.T, ctx context.Context, store *Store, convID int64) gen.AgentStep {
	t.Helper()
	steps, err := store.ListSteps(ctx, planMessageID(t, ctx, store, convID))
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	for i := range steps { // bounded by len(steps)
		if steps[i].Status == "awaiting" {
			return steps[i]
		}
	}
	t.Fatalf("no awaiting step persisted; steps=%+v", steps)
	return gen.AgentStep{}
}

// countNudgeMessages counts user messages (after the assistant stall) whose text
// names the pending write tool — i.e. nudge messages persisted by nudgeWrite.
func countNudgeMessages(t *testing.T, ctx context.Context, store *Store, convID int64) int {
	t.Helper()
	msgs, err := store.ListMessages(ctx, convID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	n := 0
	for i := range msgs {
		if msgs[i].Role != "user" {
			continue
		}
		for j := range msgs[i].Content {
			b := msgs[i].Content[j]
			if b.Type == llm.BlockText && strings.Contains(b.Text, "create_invoice") {
				n++
			}
		}
	}
	return n
}

// TestStallNudgeThenActs (case 1): the model stalls in prose once, the loop
// nudges it, and the model then calls create_invoice (which suspends). Asserts a
// nudge user message was persisted after the stall, an awaiting create_invoice
// step exists, and no invoice was created yet.
func TestStallNudgeThenActs(t *testing.T) {
	// The write suspends for approval BEFORE its handler runs, so participantId=0
	// here never reaches validation — the awaiting step is all this case checks.
	fake := llm.NewFake(
		stallPlanResp(),                      // plan: declares create_invoice
		endTurnResp("I'll create that now."), // execute turn 1: stall (end_turn)
		customCreateInvoice("tu_write", 0),   // execute turn 2: the write (suspends)
	)
	ag, store, inv, _, ctx := newStallAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "Stall nudge")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := ag.Start(ctx, conv.ID, "create an invoice"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := countNudgeMessages(t, ctx, store, conv.ID); got != 1 {
		t.Fatalf("nudge user messages = %d, want 1", got)
	}
	step := awaitingStep(t, ctx, store, conv.ID)
	if step.ToolName != "create_invoice" {
		t.Fatalf("awaiting step tool = %q, want create_invoice", step.ToolName)
	}
	rows, err := inv.List(ctx)
	if err != nil {
		t.Fatalf("inv.List: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 invoices (write suspended), got %d", len(rows))
	}
}

// TestStallNudgeStallForce (case 2): the model stalls twice. After the first
// stall the loop nudges; after the second it escalates to a forced tool call.
// Asserts the request for the forcing turn carried ToolChoice.ForceTool ==
// create_invoice, exactly one nudge message was persisted, and an awaiting
// create_invoice step exists.
func TestStallNudgeStallForce(t *testing.T) {
	fake := llm.NewFake(
		stallPlanResp(),                      // plan
		endTurnResp("Let me draft that."),    // execute turn 1: stall → nudge
		endTurnResp("Confirming details..."), // execute turn 2: stall → force
		customCreateInvoice("tu_write", 0),   // execute turn 3: forced write (suspends)
	)
	ag, store, _, _, ctx := newStallAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "Stall force")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := ag.Start(ctx, conv.ID, "create an invoice"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Requests: [0]=plan force, [1]=execute turn1, [2]=execute turn2, [3]=forced.
	if len(fake.Requests) < 4 {
		t.Fatalf("expected >=4 requests, got %d", len(fake.Requests))
	}
	if fake.Requests[0].ToolChoice.ForceTool != "propose_plan" {
		t.Fatalf("request[0] ForceTool = %q, want propose_plan", fake.Requests[0].ToolChoice.ForceTool)
	}
	if fake.Requests[1].ToolChoice.ForceTool != "" {
		t.Fatalf("request[1] (turn1) ForceTool = %q, want empty", fake.Requests[1].ToolChoice.ForceTool)
	}
	if fake.Requests[2].ToolChoice.ForceTool != "" {
		t.Fatalf("request[2] (turn2) ForceTool = %q, want empty", fake.Requests[2].ToolChoice.ForceTool)
	}
	if fake.Requests[3].ToolChoice.ForceTool != "create_invoice" {
		t.Fatalf("request[3] (forcing turn) ForceTool = %q, want create_invoice", fake.Requests[3].ToolChoice.ForceTool)
	}

	if got := countNudgeMessages(t, ctx, store, conv.ID); got != 1 {
		t.Fatalf("nudge user messages = %d, want exactly 1 (2nd stall escalates to force, no 2nd nudge)", got)
	}
	step := awaitingStep(t, ctx, store, conv.ID)
	if step.ToolName != "create_invoice" {
		t.Fatalf("awaiting step tool = %q, want create_invoice", step.ToolName)
	}
}

// TestStallHappyPath (case 3): the model calls create_invoice immediately on the
// first execute turn (no stall). Asserts no nudge user message was persisted, an
// awaiting step exists, and no execute request used ForceTool.
func TestStallHappyPath(t *testing.T) {
	fake := llm.NewFake(
		stallPlanResp(),                    // plan
		customCreateInvoice("tu_write", 0), // execute turn 1: the write (suspends)
	)
	ag, store, _, _, ctx := newStallAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "Happy path")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := ag.Start(ctx, conv.ID, "create an invoice"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := countNudgeMessages(t, ctx, store, conv.ID); got != 0 {
		t.Fatalf("nudge user messages = %d, want 0 (no stall)", got)
	}
	step := awaitingStep(t, ctx, store, conv.ID)
	if step.ToolName != "create_invoice" {
		t.Fatalf("awaiting step tool = %q, want create_invoice", step.ToolName)
	}
	// No execute request (index >=1) may force a tool.
	for i := 1; i < len(fake.Requests); i++ {
		if fake.Requests[i].ToolChoice.ForceTool != "" {
			t.Fatalf("execute request[%d] ForceTool = %q, want empty (happy path)", i, fake.Requests[i].ToolChoice.ForceTool)
		}
	}
}

// TestStallMaxBound (case 4): the model never calls the tool — it stalls on every
// execute turn. Asserts the loop terminates cleanly (Start returns nil), commits
// the checkpoint, creates no invoice, and gives up at the bound (at most one
// nudge message + one forced request) rather than looping to MaxIterations.
func TestStallMaxBound(t *testing.T) {
	fake := llm.NewFake(
		stallPlanResp(),               // plan
		endTurnResp("Still thinking"), // turn1: stall → nudge
		endTurnResp("Almost there"),   // turn2: stall → force
		endTurnResp("Here you go"),    // turn3: forced turn still no call → give up + commit
	)
	ag, store, inv, _, ctx := newStallAgent(t, fake)

	conv, err := store.CreateConversation(ctx, "Max bound")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := ag.Start(ctx, conv.ID, "create an invoice"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// No invoice (it never called the tool).
	rows, err := inv.List(ctx)
	if err != nil {
		t.Fatalf("inv.List: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 invoices, got %d", len(rows))
	}

	// It gave up at the bound: exactly one nudge message, exactly one forced
	// request, and well short of MaxIterations worth of execute calls.
	if got := countNudgeMessages(t, ctx, store, conv.ID); got != 1 {
		t.Fatalf("nudge user messages = %d, want exactly 1", got)
	}
	forced := 0
	for i := range fake.Requests {
		if fake.Requests[i].ToolChoice.ForceTool == "create_invoice" {
			forced++
		}
	}
	if forced != 1 {
		t.Fatalf("forced create_invoice requests = %d, want exactly 1 (bounded escalation)", forced)
	}
	// Total model calls: plan + 3 execute turns = 4. Must not have spun to the
	// MaxIterations cap.
	if len(fake.Requests) != 4 {
		t.Fatalf("model calls = %d, want 4 (plan + 3 execute, gave up at bound)", len(fake.Requests))
	}

	// The checkpoint was committed (the turn ended cleanly, not via error).
	cp, err := store.GetCheckpoint(ctx, 1)
	if err != nil {
		t.Fatalf("GetCheckpoint: %v", err)
	}
	if cp.Status != "committed" {
		t.Fatalf("checkpoint status = %q, want committed", cp.Status)
	}
}

// TestStallForceThenApproveCreates (case 5, end-to-end): reuse case 2's
// stall→nudge→stall→force script with a valid custom line, then approve the
// awaiting step via Decide and assert an invoice is created.
func TestStallForceThenApproveCreates(t *testing.T) {
	ag, store, inv, _, ctx := newStallAgentDeferred(t)

	conv, err := store.CreateConversation(ctx, "Force then approve")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := ag.Start(ctx, conv.ID, "create an invoice"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	step := awaitingStep(t, ctx, store, conv.ID)
	if step.ToolName != "create_invoice" {
		t.Fatalf("awaiting step tool = %q, want create_invoice", step.ToolName)
	}

	// No invoice before approval.
	if rows, lErr := inv.List(ctx); lErr != nil {
		t.Fatalf("inv.List: %v", lErr)
	} else if len(rows) != 0 {
		t.Fatalf("expected 0 invoices before approval, got %d", len(rows))
	}

	if err := ag.Decide(ctx, step.ID, true); err != nil {
		t.Fatalf("Decide(allow): %v", err)
	}

	rows, err := inv.List(ctx)
	if err != nil {
		t.Fatalf("inv.List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 invoice after approval, got %d", len(rows))
	}
}

// newStallAgentDeferred builds the stall agent first (to learn the participant
// id), then scripts the forced-path fake against that id and swaps it onto the
// agent. This is needed because the create_invoice input must reference the
// seeded participant, which is only known after wiring the stack.
func newStallAgentDeferred(t *testing.T) (*Agent, *Store, *service.InvoiceService, int64, context.Context) {
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

	p, err := participant.NewParticipants(conn).Create(ctx, tenantID, participant.ParticipantInput{
		Name: "Approve Participant", PlanStart: "2025-07-01", PlanEnd: "2026-06-30",
	})
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}

	fake := llm.NewFake(
		stallPlanResp(),
		endTurnResp("Let me draft that."),
		endTurnResp("Confirming details..."),
		customCreateInvoice("tu_write", p.ID), // valid custom line for THIS participant
		// After approval, Decide feeds the tool_result and resumes the loop; the
		// model gets one more turn to wrap up. The write has now been attempted,
		// so this end_turn commits cleanly without re-triggering the stall path.
		endTurnResp("Done — invoice drafted for your approval."),
	)

	store := NewStore(conn)
	hub := realtime.NewHub()
	inv := service.NewInvoiceService(conn, hub)
	cp := NewCheckpoint(store, conn)

	reg := NewRegistry()
	reg.Register(NewListInvoicesTool(inv))
	reg.Register(NewCreateInvoiceTool(inv, cp))

	events := NewEvents()
	cfg := Config{APIKey: "test", MaxIterations: 8}.WithDefaults()
	ag := NewAgent(cfg, fake, store, reg, cp, events)
	return ag, store, inv, p.ID, ctx
}
