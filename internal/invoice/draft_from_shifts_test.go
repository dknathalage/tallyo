package invoice

import (
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/shift"
)

// recordedShiftWithItems seeds a recorded, unbilled shift carrying nItems
// pre-priced custom line items (qty 1, the given unit price each) and returns
// its id. Items are created through the shift repo so no catalogue seeding is
// needed (custom lines keep their supplied price).
func recordedShiftWithItems(t *testing.T, shiftSvc *shift.Service, repo *shift.ShiftsRepo, tenantID, participantID int64, date string, prices ...float64) int64 {
	t.Helper()
	ctx := tctx(tenantID)
	sh, err := shiftSvc.Create(ctx, shift.ShiftInput{ParticipantID: participantID, ServiceDate: date, Status: "recorded"})
	if err != nil {
		t.Fatalf("recordedShiftWithItems create: %v", err)
	}
	for i := range prices { // bounded by len(prices)
		in := billing.LineItemInput{Description: "work", Unit: "EA", Quantity: 1, UnitPrice: prices[i]}
		if _, err := repo.CreateItem(ctx, tenantID, sh.ID, in); err != nil {
			t.Fatalf("recordedShiftWithItems CreateItem: %v", err)
		}
	}
	return sh.ID
}

// newDraftHarness builds an invoice service wired to a real *shift.Service over
// one shared connection, plus the shift repo for seeding.
func newDraftHarness(t *testing.T) (*Service, *shift.Service, *shift.ShiftsRepo, int64, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	participantID := seedParticipant(t, conn, tenantID, "Jane Participant")
	hub := realtime.NewHub()
	shiftSvc := shift.NewService(conn, hub, NewInvoices(conn))
	invSvc := NewService(conn, hub, shiftSvc)
	return invSvc, shiftSvc, shift.NewShifts(conn), tenantID, participantID
}

func TestDraftFromShiftsLinksItemsAndTotals(t *testing.T) {
	invSvc, shiftSvc, repo, tenantID, participantID := newDraftHarness(t)
	ctx := tctx(tenantID)

	s1 := recordedShiftWithItems(t, shiftSvc, repo, tenantID, participantID, "2026-01-10", 10, 20)
	s2 := recordedShiftWithItems(t, shiftSvc, repo, tenantID, participantID, "2026-01-11", 30, 40)

	inv, err := invSvc.DraftFromShifts(ctx, []int64{s1, s2})
	if err != nil {
		t.Fatalf("DraftFromShifts: %v", err)
	}
	if inv == nil {
		t.Fatal("DraftFromShifts returned nil invoice")
	}
	if inv.ParticipantID != participantID {
		t.Fatalf("invoice participant = %d, want %d", inv.ParticipantID, participantID)
	}
	if len(inv.LineItems) != 4 {
		t.Fatalf("invoice line items = %d, want 4", len(inv.LineItems))
	}
	for _, li := range inv.LineItems {
		if li.InvoiceID == nil || *li.InvoiceID != inv.ID {
			t.Fatalf("line item not linked to invoice: %+v", li)
		}
		if li.ShiftID == nil {
			t.Fatalf("shift item lost its shift_id: %+v", li)
		}
	}
	if inv.Subtotal != 100 || inv.Total != 100 {
		t.Fatalf("invoice subtotal=%v total=%v, want 100/100", inv.Subtotal, inv.Total)
	}

	for _, sid := range []int64{s1, s2} {
		sh, _ := shiftSvc.Get(ctx, sid)
		if sh == nil || sh.Status != "drafted" || sh.InvoiceID == nil || *sh.InvoiceID != inv.ID {
			t.Fatalf("shift %d after draft = %+v, want drafted + invoice %d", sid, sh, inv.ID)
		}
	}
}

func TestDraftFromShiftsEmptyShiftErrors(t *testing.T) {
	invSvc, shiftSvc, repo, tenantID, participantID := newDraftHarness(t)
	ctx := tctx(tenantID)

	withItems := recordedShiftWithItems(t, shiftSvc, repo, tenantID, participantID, "2026-01-10", 10)
	// A recorded shift with zero items (G5).
	empty, err := shiftSvc.Create(ctx, shift.ShiftInput{ParticipantID: participantID, ServiceDate: "2026-01-11", Status: "recorded"})
	if err != nil {
		t.Fatalf("create empty shift: %v", err)
	}

	if _, err := invSvc.DraftFromShifts(ctx, []int64{withItems, empty.ID}); err == nil {
		t.Fatal("DraftFromShifts with an empty shift must error (G5)")
	}
	// Nothing should have been linked: the non-empty shift stays recorded.
	sh, _ := shiftSvc.Get(ctx, withItems)
	if sh == nil || sh.Status != "recorded" || sh.InvoiceID != nil {
		t.Fatalf("shift must stay recorded after a failed draft, got %+v", sh)
	}
}

func TestDraftFromShiftsSingleParticipantGuard(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T2")
	p1 := seedParticipant(t, conn, tid, "P1")
	p2 := seedParticipant(t, conn, tid, "P2")
	hub := realtime.NewHub()
	shSvc := shift.NewService(conn, hub, NewInvoices(conn))
	iSvc := NewService(conn, hub, shSvc)
	r := shift.NewShifts(conn)

	a := recordedShiftWithItems(t, shSvc, r, tid, p1, "2026-01-10", 10)
	b := recordedShiftWithItems(t, shSvc, r, tid, p2, "2026-01-11", 20)

	if _, err := iSvc.DraftFromShifts(tctx(tid), []int64{a, b}); err == nil {
		t.Fatal("DraftFromShifts across two participants must error")
	}

	// And a clean single-participant pair still drafts.
	c := recordedShiftWithItems(t, shSvc, r, tid, p1, "2026-01-12", 5)
	if _, err := iSvc.DraftFromShifts(tctx(tid), []int64{c}); err != nil {
		t.Fatalf("single-participant DraftFromShifts: %v", err)
	}
}
