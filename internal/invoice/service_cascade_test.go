package invoice

import (
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/shift"
)

// seedDraftedShift creates a recorded shift and drafts it onto inv via the
// shift.Service, returning the shift id.
func seedDraftedShift(t *testing.T, shiftSvc *shift.Service, tenantID, clientID, invoiceID int64) int64 {
	t.Helper()
	ctx := tctx(tenantID)
	sh, err := shiftSvc.Create(ctx, shift.ShiftInput{ClientID: clientID, ServiceDate: "2026-01-15"})
	if err != nil {
		t.Fatalf("seedDraftedShift create: %v", err)
	}
	if err := shiftSvc.MarkDrafted(ctx, invoiceID, []int64{sh.ID}); err != nil {
		t.Fatalf("seedDraftedShift MarkDrafted: %v", err)
	}
	return sh.ID
}

func TestInvoiceStatusCascadesToShifts(t *testing.T) {
	for _, status := range []string{"sent", "paid"} {
		t.Run(status, func(t *testing.T) {
			conn := newTestDB(t)
			tenantID := seedTenant(t, conn, "Acme NDIS")
			clientID := seedClient(t, conn, tenantID, "Jane Client")
			hub := realtime.NewHub()
			invSvc := NewService(conn, conn, hub, shift.NewService(conn, conn, hub, NewInvoices(conn)))
			shiftSvc := shift.NewService(conn, conn, hub, NewInvoices(conn))
			ctx := tctx(tenantID)

			inv, err := invSvc.Create(ctx, InvoiceInput{
				ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
			}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
			if err != nil {
				t.Fatalf("Create invoice: %v", err)
			}
			shiftID := seedDraftedShift(t, shiftSvc, tenantID, clientID, inv.ID)

			if err := invSvc.UpdateStatus(ctx, inv.ID, status); err != nil {
				t.Fatalf("UpdateStatus %s: %v", status, err)
			}
			got, _ := shiftSvc.Get(ctx, shiftID)
			if got == nil || got.Status != status {
				t.Fatalf("shift status after invoice %s = %+v, want %s", status, got, status)
			}
		})
	}
}

func TestInvoiceStatusDoesNotCascadeForDraft(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	hub := realtime.NewHub()
	invSvc := NewService(conn, conn, hub, shift.NewService(conn, conn, hub, NewInvoices(conn)))
	shiftSvc := shift.NewService(conn, conn, hub, NewInvoices(conn))
	ctx := tctx(tenantID)

	inv, err := invSvc.Create(ctx, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create invoice: %v", err)
	}
	shiftID := seedDraftedShift(t, shiftSvc, tenantID, clientID, inv.ID)

	// A non-terminal status (e.g. back to draft) must NOT advance the shift.
	if err := invSvc.UpdateStatus(ctx, inv.ID, "draft"); err != nil {
		t.Fatalf("UpdateStatus draft: %v", err)
	}
	got, _ := shiftSvc.Get(ctx, shiftID)
	if got == nil || got.Status != "drafted" {
		t.Fatalf("shift should stay drafted on invoice draft status, got %+v", got)
	}
}

func TestInvoiceDeleteRevertsShiftsToRecorded(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	hub := realtime.NewHub()
	invSvc := NewService(conn, conn, hub, shift.NewService(conn, conn, hub, NewInvoices(conn)))
	shiftSvc := shift.NewService(conn, conn, hub, NewInvoices(conn))
	ctx := tctx(tenantID)

	inv, err := invSvc.Create(ctx, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create invoice: %v", err)
	}
	shiftID := seedDraftedShift(t, shiftSvc, tenantID, clientID, inv.ID)

	if err := invSvc.Delete(ctx, inv.ID); err != nil {
		t.Fatalf("Delete invoice: %v", err)
	}
	got, _ := shiftSvc.Get(ctx, shiftID)
	if got == nil || got.Status != "recorded" || got.InvoiceID != nil {
		t.Fatalf("shift after invoice delete = %+v, want recorded + nil invoice", got)
	}
}
