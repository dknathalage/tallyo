package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

// newCheckpointFixture builds a Store, an InvoiceService and an open checkpoint
// tied to a real assistant message, plus a seeded participant id.
func newCheckpointFixture(t *testing.T) (ctx context.Context, s *Store, inv *service.InvoiceService, cp *Checkpoint, checkpointID, participantID int64) {
	t.Helper()
	s = newTestStore(t)
	tenantID, userID := seedTenantUser(t, s.db)
	ctx = reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)

	participantID = seedAgentParticipant(t, s.db, ctx)

	inv = service.NewInvoiceService(s.db, realtime.NewHub())
	cp = NewCheckpoint(s, s.db)

	conv, err := s.CreateConversation(ctx, "chat")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	msg, err := s.CreateMessage(ctx, conv.ID, "assistant", nil, "{}")
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	checkpointID, err = cp.Open(ctx, msg.ID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return ctx, s, inv, cp, checkpointID, participantID
}

// seedAgentParticipant inserts a participant via the repository in the agent
// test package (which has no seedParticipant helper of its own).
func seedAgentParticipant(t *testing.T, db *sql.DB, ctx context.Context) int64 {
	t.Helper()
	p, err := repository.NewParticipants(db).Create(ctx, reqctx.MustTenant(ctx), repository.ParticipantInput{Name: "Jane Participant"})
	if err != nil {
		t.Fatalf("seedAgentParticipant: %v", err)
	}
	return p.ID
}

func TestCheckpointRevertCreateDeletesRow(t *testing.T) {
	ctx, s, inv, cp, checkpointID, participantID := newCheckpointFixture(t)

	created, err := inv.Create(ctx, repository.InvoiceInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []repository.LineItemInput{{Description: "Custom A", Quantity: 2, UnitPrice: 10}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	after, _ := json.Marshal(created)
	if err := cp.Record(ctx, checkpointID, 1, Change{
		Table: "invoices", PK: created.ID, Op: "create",
		AfterRow: after, EntityVersion: created.UpdatedAt,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	conflicts, err := cp.Revert(ctx, checkpointID, InvoiceRestoreFunc(inv))
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("Revert: got %d conflicts, want 0", len(conflicts))
	}

	got, err := inv.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatalf("invoice %d still exists after revert", created.ID)
	}

	chk, err := s.GetCheckpoint(ctx, checkpointID)
	if err != nil {
		t.Fatalf("GetCheckpoint: %v", err)
	}
	if chk.Status != "reverted" {
		t.Fatalf("checkpoint status = %q, want reverted", chk.Status)
	}
}

func TestCheckpointRevertConflictReportedNotApplied(t *testing.T) {
	ctx, _, inv, cp, checkpointID, participantID := newCheckpointFixture(t)

	created, err := inv.Create(ctx, repository.InvoiceInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []repository.LineItemInput{{Description: "Custom A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	after, _ := json.Marshal(created)
	// Stale entity version: does NOT match the live row's UpdatedAt.
	if err := cp.Record(ctx, checkpointID, 1, Change{
		Table: "invoices", PK: created.ID, Op: "create",
		AfterRow: after, EntityVersion: "1999-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	conflicts, err := cp.Revert(ctx, checkpointID, InvoiceRestoreFunc(inv))
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].PK != created.ID {
		t.Fatalf("conflicts = %+v, want one for pk %d", conflicts, created.ID)
	}

	// The row must still exist (the conflict was not applied).
	got, err := inv.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatalf("invoice %d was deleted despite conflict", created.ID)
	}
}

func TestCreateInvoiceToolRecordsCheckpoint(t *testing.T) {
	ctx, s, inv, cp, checkpointID, participantID := newCheckpointFixture(t)
	ctx = withCheckpoint(ctx, checkpointID)

	tool := NewCreateInvoiceTool(inv, cp)
	if tool.Risk != RiskRisky {
		t.Fatalf("Risk = %q, want risky", tool.Risk)
	}

	input, _ := json.Marshal(map[string]any{
		"participantId": participantID,
		"items":         []map[string]any{{"description": "Custom A", "quantity": 2, "unitPrice": 10}},
	})
	res, err := tool.Handler(ctx, input)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	out, ok := res.JSON.(*repository.Invoice)
	if !ok || out == nil {
		t.Fatalf("Handler JSON = %T, want *repository.Invoice", res.JSON)
	}

	changes, err := s.ListCheckpointChanges(ctx, checkpointID)
	if err != nil {
		t.Fatalf("ListCheckpointChanges: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("recorded %d changes, want 1", len(changes))
	}
	if changes[0].Pk != out.ID || changes[0].Op != "create" {
		t.Fatalf("change = %+v, want create for pk %d", changes[0], out.ID)
	}
}

func TestCreateInvoiceToolValidationError(t *testing.T) {
	ctx, _, inv, cp, checkpointID, participantID := newCheckpointFixture(t)
	ctx = withCheckpoint(ctx, checkpointID)
	tool := NewCreateInvoiceTool(inv, cp)

	// A support-item line with a bogus code triggers the NDIS validator.
	input, _ := json.Marshal(map[string]any{
		"participantId": participantID,
		"items": []map[string]any{
			{"code": "NOPE-99", "serviceDate": "2026-01-01", "quantity": 1, "unitPrice": 5},
		},
	})
	_, err := tool.Handler(ctx, input)
	if err == nil {
		t.Fatalf("Handler: want validation error, got nil")
	}
}
