package agent

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// TestSweepExpired proves the expired-awaiting-step sweep: an awaiting step
// whose await_expires_at is in the past is marked denied, its checkpoint is
// committed (so prior allowed changes in the turn stand), and a step_expired
// event fires on the step's conversation.
func TestSweepExpired(t *testing.T) {
	fake := llm.NewFake()
	ag, store, _, ctx := newTestAgent(t, fake)

	// Pin the agent's clock so "now" is deterministic and after the step's
	// await_expires_at below.
	clk := &fakeClock{now: time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)}
	ag.clock = clk

	conv, err := store.CreateConversation(ctx, "Sweep test")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	msg, err := store.CreateMessage(ctx, conv.ID, "assistant",
		[]llm.Block{{Type: llm.BlockText, Text: "plan"}}, "{}")
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	checkpointID, err := ag.cp.Open(ctx, msg.ID)
	if err != nil {
		t.Fatalf("checkpoint open: %v", err)
	}

	// An awaiting step that expired one hour before the agent's clock.
	expired := clk.now.Add(-time.Hour).UTC().Format(time.RFC3339)
	step, err := store.CreateAwaitingStep(ctx, gen.CreateAwaitingStepParams{
		MessageID:      msg.ID,
		CheckpointID:   sqlInt64(checkpointID),
		Ordinal:        0,
		ToolName:       "create_invoice",
		ToolUseID:      "tu_expired",
		Summary:        "Run create_invoice",
		Risk:           string(RiskRisky),
		PendingInput:   `{}`,
		AwaitExpiresAt: sqlString(expired),
	})
	if err != nil {
		t.Fatalf("CreateAwaitingStep: %v", err)
	}

	// Subscribe before sweeping to capture the step_expired event.
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

	if err := ag.SweepExpired(ctx); err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}
	unsub()
	<-done

	// Step is denied.
	gotStep, err := store.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if gotStep.Status != "denied" {
		t.Fatalf("step status = %q, want denied", gotStep.Status)
	}

	// Checkpoint is committed.
	cp, err := store.GetCheckpoint(ctx, checkpointID)
	if err != nil {
		t.Fatalf("GetCheckpoint: %v", err)
	}
	if cp.Status != "committed" {
		t.Fatalf("checkpoint status = %q, want committed", cp.Status)
	}

	// A step_expired event fired.
	var sawExpired bool
	for _, ev := range got {
		if ev.Type == "step_expired" {
			sawExpired = true
		}
	}
	if !sawExpired {
		t.Fatalf("expected a step_expired event; got %+v", got)
	}
}

// TestSweepExpiredSkipsFresh proves a not-yet-expired awaiting step is left
// untouched by the sweep.
func TestSweepExpiredSkipsFresh(t *testing.T) {
	fake := llm.NewFake()
	ag, store, _, ctx := newTestAgent(t, fake)

	clk := &fakeClock{now: time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)}
	ag.clock = clk

	conv, err := store.CreateConversation(ctx, "Fresh test")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	msg, err := store.CreateMessage(ctx, conv.ID, "assistant",
		[]llm.Block{{Type: llm.BlockText, Text: "plan"}}, "{}")
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	checkpointID, err := ag.cp.Open(ctx, msg.ID)
	if err != nil {
		t.Fatalf("checkpoint open: %v", err)
	}

	// Expires one hour AFTER the clock — still valid.
	fresh := clk.now.Add(time.Hour).UTC().Format(time.RFC3339)
	step, err := store.CreateAwaitingStep(ctx, gen.CreateAwaitingStepParams{
		MessageID:      msg.ID,
		CheckpointID:   sqlInt64(checkpointID),
		Ordinal:        0,
		ToolName:       "create_invoice",
		ToolUseID:      "tu_fresh",
		Summary:        "Run create_invoice",
		Risk:           string(RiskRisky),
		PendingInput:   `{}`,
		AwaitExpiresAt: sqlString(fresh),
	})
	if err != nil {
		t.Fatalf("CreateAwaitingStep: %v", err)
	}

	if err := ag.SweepExpired(ctx); err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}

	gotStep, err := store.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if gotStep.Status != "awaiting" {
		t.Fatalf("fresh step status = %q, want awaiting (untouched)", gotStep.Status)
	}
}
