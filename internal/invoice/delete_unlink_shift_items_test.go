package invoice

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/google/uuid"
)

// addManualLine inserts a manual (shift-less) line onto an invoice — a line that
// SHOULD vanish when the invoice is deleted (it has no shift to return to).
func addManualLine(t *testing.T, conn *sql.DB, tenantID, invoiceID int64, price float64) {
	t.Helper()
	_, err := gen.New(conn).CreateLineItem(context.Background(), gen.CreateLineItemParams{
		Uuid:        uuid.NewString(),
		TenantID:    tenantID,
		InvoiceID:   sql.NullInt64{Int64: invoiceID, Valid: true},
		Description: "manual",
		Quantity:    1,
		UnitPrice:   price,
		LineTotal:   price,
		SortOrder:   sql.NullInt64{Int64: 99, Valid: true},
	})
	if err != nil {
		t.Fatalf("addManualLine: %v", err)
	}
}

// shiftItemSurvival reports (unbilled shift items for shift, total line items for
// the (now possibly gone) invoice id).
func countShiftItems(t *testing.T, conn *sql.DB, tenantID, shiftID int64) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM line_items WHERE tenant_id=? AND shift_id=? AND invoice_id IS NULL`,
		tenantID, shiftID).Scan(&n); err != nil {
		t.Fatalf("countShiftItems: %v", err)
	}
	return n
}

func countInvoiceLines(t *testing.T, conn *sql.DB, tenantID, invoiceID int64) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM line_items WHERE tenant_id=? AND invoice_id=?`,
		tenantID, invoiceID).Scan(&n); err != nil {
		t.Fatalf("countInvoiceLines: %v", err)
	}
	return n
}

func TestDeleteUnlinksShiftItemsBeforeCascade(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "T")
	participantID := seedParticipant(t, conn, tenantID, "Jane")
	hub := realtime.NewHub()
	shiftSvc := shift.NewService(conn, conn, hub, NewInvoices(conn))
	invSvc := NewService(conn, conn, hub, shiftSvc)
	repo := shift.NewShifts(conn)
	ctx := tctx(tenantID)

	sid := recordedShiftWithItems(t, shiftSvc, repo, tenantID, participantID, "2026-01-10", 10, 20)
	inv, err := invSvc.DraftFromShifts(ctx, []int64{sid})
	if err != nil {
		t.Fatalf("DraftFromShifts: %v", err)
	}
	addManualLine(t, conn, tenantID, inv.ID, 5)

	if got := countInvoiceLines(t, conn, tenantID, inv.ID); got != 3 {
		t.Fatalf("invoice lines before delete = %d, want 3 (2 shift + 1 manual)", got)
	}

	if err := invSvc.Delete(ctx, inv.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if got := countShiftItems(t, conn, tenantID, sid); got != 2 {
		t.Fatalf("shift items after delete = %d, want 2 (survived, unlinked)", got)
	}
	if got := countInvoiceLines(t, conn, tenantID, inv.ID); got != 0 {
		t.Fatalf("invoice lines after delete = %d, want 0 (manual line cascaded away)", got)
	}
	sh, _ := shiftSvc.Get(ctx, sid)
	if sh == nil || sh.Status != "recorded" || sh.InvoiceID != nil {
		t.Fatalf("shift after delete = %+v, want recorded + nil invoice", sh)
	}
}

func TestBulkDeleteUnlinksShiftItemsBeforeCascade(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "T")
	participantID := seedParticipant(t, conn, tenantID, "Jane")
	hub := realtime.NewHub()
	shiftSvc := shift.NewService(conn, conn, hub, NewInvoices(conn))
	invSvc := NewService(conn, conn, hub, shiftSvc)
	repo := shift.NewShifts(conn)
	ctx := tctx(tenantID)

	s1 := recordedShiftWithItems(t, shiftSvc, repo, tenantID, participantID, "2026-01-10", 10)
	s2 := recordedShiftWithItems(t, shiftSvc, repo, tenantID, participantID, "2026-01-11", 20)
	inv1, err := invSvc.DraftFromShifts(ctx, []int64{s1})
	if err != nil {
		t.Fatalf("draft inv1: %v", err)
	}
	inv2, err := invSvc.DraftFromShifts(ctx, []int64{s2})
	if err != nil {
		t.Fatalf("draft inv2: %v", err)
	}
	addManualLine(t, conn, tenantID, inv1.ID, 5)
	addManualLine(t, conn, tenantID, inv2.ID, 7)

	if err := invSvc.BulkDelete(ctx, []int64{inv1.ID, inv2.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}

	for _, sid := range []int64{s1, s2} {
		if got := countShiftItems(t, conn, tenantID, sid); got != 1 {
			t.Fatalf("shift %d items after bulk delete = %d, want 1", sid, got)
		}
		sh, _ := shiftSvc.Get(ctx, sid)
		if sh == nil || sh.Status != "recorded" || sh.InvoiceID != nil {
			t.Fatalf("shift %d after bulk delete = %+v, want recorded + nil invoice", sid, sh)
		}
	}
	if got := countInvoiceLines(t, conn, tenantID, inv1.ID) + countInvoiceLines(t, conn, tenantID, inv2.ID); got != 0 {
		t.Fatalf("manual lines should have cascaded away, %d remain", got)
	}
}
