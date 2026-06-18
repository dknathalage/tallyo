package agent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/shift"
)

// startSuspendedRisky builds a fresh agent over a real DB with a seeded
// participant, scripts the Fake through Start so the loop suspends on a risky
// create_invoice (a CUSTOM line item that passes validation), and returns the
// wired agent/store/inv/ctx, the conversation id, the awaiting step id, and the
// Fake (still holding the appended resume responses). The participant is seeded
// BEFORE scripting so the risky input references a real participant id.
func startSuspendedRisky(t *testing.T, resume ...llm.Response) (*Agent, *Store, *invoice.Service, context.Context, int64, int64, *llm.Fake) {
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
	inv := invoice.NewService(conn, realtime.NewHub(), shift.NewShifts(conn))
	cp := NewCheckpoint(store, conn)

	participantID := seedAgentParticipant(t, conn, ctx)
	riskyInput := fmt.Sprintf(
		`{"participantId":%d,"issueDate":"2026-01-01","dueDate":"2026-02-01","items":[{"description":"Custom A","quantity":2,"unitPrice":10}]}`,
		participantID)

	scripted := []llm.Response{
		// 1: plan turn (forced propose_plan)
		toolUseResp(llm.StopToolUse, "propose_plan", "tu_plan",
			`{"steps":[{"tool":"create_invoice","summary":"make one","risk":"risky"}]}`),
		// 2: execute turn — risky create_invoice tool_use (suspends here)
		toolUseResp(llm.StopToolUse, "create_invoice", "tu_risky", riskyInput),
	}
	scripted = append(scripted, resume...)
	fake := llm.NewFake(scripted...)

	reg := NewRegistry()
	reg.Register(NewListInvoicesTool(inv))
	reg.Register(NewCreateInvoiceTool(inv, cp))
	cfg := Config{APIKey: "test", MaxIterations: 5}.WithDefaults()
	ag := NewAgent(cfg, fake, store, reg, cp, NewEvents())

	conv, err := store.CreateConversation(ctx, "Decide test")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := ag.Start(ctx, conv.ID, "create an invoice"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stepID := awaitingStepID(t, ctx, store, conv.ID)
	return ag, store, inv, ctx, conv.ID, stepID, fake
}

// awaitingStepID returns the id of the single awaiting step in the conversation.
func awaitingStepID(t *testing.T, ctx context.Context, store *Store, convID int64) int64 {
	t.Helper()
	msgID := planMessageID(t, ctx, store, convID)
	steps, err := store.ListSteps(ctx, msgID)
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	for i := range steps {
		if steps[i].Status == "awaiting" {
			return steps[i].ID
		}
	}
	t.Fatalf("no awaiting step; steps=%+v", steps)
	return 0
}

func TestDecideAllowCreatesAndResumes(t *testing.T) {
	ag, store, inv, ctx, convID, stepID, fake := startSuspendedRisky(t,
		// 3: resume turn — end turn after the approved write
		llm.Response{StopReason: llm.StopEndTurn, Content: []llm.Block{{Type: llm.BlockText, Text: "invoice created"}}},
	)

	// Sanity: no invoice yet (suspend, not run).
	if rows, err := inv.List(ctx); err != nil {
		t.Fatalf("inv.List: %v", err)
	} else if len(rows) != 0 {
		t.Fatalf("expected 0 invoices before Decide, got %d", len(rows))
	}

	ch, unsub := ag.events.Subscribe(convID)
	defer unsub()
	var got []Event
	done := make(chan struct{})
	go func() {
		for ev := range ch {
			got = append(got, ev)
		}
		close(done)
	}()

	if err := ag.Decide(ctx, stepID, true); err != nil {
		t.Fatalf("Decide(allow): %v", err)
	}
	unsub()
	<-done

	// An invoice now exists.
	rows, err := inv.List(ctx)
	if err != nil {
		t.Fatalf("inv.List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 invoice after allow, got %d", len(rows))
	}

	// Step is terminal (done) and a checkpoint change was recorded for the invoice.
	step, err := store.GetStep(ctx, stepID)
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if step.Status != "done" {
		t.Fatalf("step status = %q, want done", step.Status)
	}
	changes, err := store.ListCheckpointChanges(ctx, step.CheckpointID.Int64)
	if err != nil {
		t.Fatalf("ListCheckpointChanges: %v", err)
	}
	if len(changes) != 1 || changes[0].TableName != "invoices" || changes[0].Op != "create" {
		t.Fatalf("expected 1 invoices/create checkpoint change, got %+v", changes)
	}

	// The loop resumed to a final message.
	var sawFinal bool
	for _, ev := range got {
		if ev.Type == "message_final" {
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Fatalf("expected message_final after resume; got %+v", got)
	}

	// The first model call AFTER Decide used AUTO tool choice (not a re-plan)
	// and its last message was the create_invoice success tool_result.
	if len(fake.Requests) != 3 {
		t.Fatalf("expected 3 model calls (plan + execute + resume), got %d", len(fake.Requests))
	}
	resumeReq := fake.Requests[2]
	// Guard BUG 1 on the resume path too: every tool_use in the window is answered.
	assertBalanced(t, resumeReq.Messages)
	if resumeReq.ToolChoice.ForceTool != "" {
		t.Fatalf("resume ForceTool = %q, want empty (auto)", resumeReq.ToolChoice.ForceTool)
	}
	if len(resumeReq.Messages) == 0 {
		t.Fatalf("resume request had no messages")
	}
	last := resumeReq.Messages[len(resumeReq.Messages)-1]
	if len(last.ToolResults) != 1 || last.ToolResults[0].ToolUseID != "tu_risky" || last.ToolResults[0].IsError {
		t.Fatalf("resume last message was not the success tool_result: %+v", last)
	}
}

func TestDecideDenyDoesNotCreate(t *testing.T) {
	ag, store, inv, ctx, _, stepID, fake := startSuspendedRisky(t,
		// 3: resume turn — model wraps up after the denial
		llm.Response{StopReason: llm.StopEndTurn, Content: []llm.Block{{Type: llm.BlockText, Text: "ok, not creating it"}}},
	)

	if err := ag.Decide(ctx, stepID, false); err != nil {
		t.Fatalf("Decide(deny): %v", err)
	}

	// No invoice created.
	rows, err := inv.List(ctx)
	if err != nil {
		t.Fatalf("inv.List: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 invoices after deny, got %d", len(rows))
	}

	// Step is denied.
	step, err := store.GetStep(ctx, stepID)
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if step.Status != "denied" {
		t.Fatalf("step status = %q, want denied", step.Status)
	}

	// An is_error tool_result was fed for tu_risky, and the loop resumed (auto).
	if len(fake.Requests) != 3 {
		t.Fatalf("expected 3 model calls (plan + execute + resume), got %d", len(fake.Requests))
	}
	resumeReq := fake.Requests[2]
	if resumeReq.ToolChoice.ForceTool != "" {
		t.Fatalf("resume ForceTool = %q, want empty (auto)", resumeReq.ToolChoice.ForceTool)
	}
	last := resumeReq.Messages[len(resumeReq.Messages)-1]
	if len(last.ToolResults) != 1 || last.ToolResults[0].ToolUseID != "tu_risky" || !last.ToolResults[0].IsError {
		t.Fatalf("resume last message was not the is_error tool_result: %+v", last)
	}
}

func TestDecideIdempotent(t *testing.T) {
	ag, _, inv, ctx, _, stepID, _ := startSuspendedRisky(t,
		llm.Response{StopReason: llm.StopEndTurn, Content: []llm.Block{{Type: llm.BlockText, Text: "invoice created"}}},
	)

	if err := ag.Decide(ctx, stepID, true); err != nil {
		t.Fatalf("first Decide(allow): %v", err)
	}

	// A second decision must be a no-op error and must NOT double-run the tool.
	if err := ag.Decide(ctx, stepID, true); !errors.Is(err, ErrStepResolved) {
		t.Fatalf("second Decide error = %v, want ErrStepResolved", err)
	}
	rows, err := inv.List(ctx)
	if err != nil {
		t.Fatalf("inv.List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected still 1 invoice (no double-run), got %d", len(rows))
	}
}
