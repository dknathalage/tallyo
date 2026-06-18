package agent

// Tests for the SkipPlan efficiency path: when Config.SkipPlan is set, Start
// omits the forced propose_plan round-trip and enters the execute loop directly.
// (Stall recovery is plan-driven, so it is intentionally inactive under SkipPlan
// — see Execute's pendingWrite comment.)

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

// newSkipPlanAgent mirrors newStallAgent but sets cfg.SkipPlan = true.
func newSkipPlanAgent(t *testing.T, client llm.Client) (*Agent, *Store, *service.InvoiceService, int64, context.Context) {
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
		Name: "SkipPlan Participant", PlanStart: "2025-07-01", PlanEnd: "2026-06-30",
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
	reg.Register(NewCreateInvoiceTool(inv, cp)) // the sole risky tool

	cfg := Config{APIKey: "test", MaxIterations: 8, SkipPlan: true}.WithDefaults()
	ag := NewAgent(cfg, client, store, reg, cp, NewEvents())
	return ag, store, inv, p.ID, ctx
}

// TestSkipPlanNoForcedPlanTurn asserts that with SkipPlan the first model call is
// an ordinary (auto) execute turn — never a forced propose_plan — and a risky
// tool still suspends for approval.
func TestSkipPlanNoForcedPlanTurn(t *testing.T) {
	fake := llm.NewFake(
		// First (and only) execute turn: immediately call the risky write.
		customCreateInvoice("tu_create", 0), // participantID patched below
	)
	ag, store, _, pid, ctx := newSkipPlanAgent(t, fake)
	// Rebuild the scripted response now that we know the participant id.
	fake.SetResponses(customCreateInvoice("tu_create", pid))

	conv, err := store.CreateConversation(ctx, "skipplan")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := ag.Start(ctx, conv.ID, "make an invoice"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(fake.Requests) == 0 {
		t.Fatal("no model calls made")
	}
	for i := range fake.Requests { // bounded
		if fake.Requests[i].ToolChoice.ForceTool == "propose_plan" {
			t.Fatalf("call %d forced propose_plan; SkipPlan must skip the plan turn", i)
		}
	}
	// The write suspended (risky), so an awaiting step exists.
	steps, err := store.ListExpiredAwaitingSteps(ctx, "2999-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("ListExpiredAwaitingSteps: %v", err)
	}
	if len(steps) != 1 || steps[0].ToolName != "create_invoice" {
		t.Fatalf("expected one awaiting create_invoice step, got %+v", steps)
	}
}
