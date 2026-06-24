package invoice

import (
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/session"
)

// recordedSessionWithItems seeds a recorded, unbilled session carrying nItems
// pre-priced custom line items (qty 1, the given unit price each) and returns
// its id. Items are created through the session repo so no catalogue seeding is
// needed (custom lines keep their supplied price).
func recordedSessionWithItems(t *testing.T, sessionSvc *session.Service, repo *session.SessionsRepo, tenantID, clientID string, date string, prices ...float64) string {
	t.Helper()
	ctx := tctx(tenantID)
	sh, err := sessionSvc.Create(ctx, session.SessionInput{ClientID: clientID, ServiceDate: date, Status: "recorded"})
	if err != nil {
		t.Fatalf("recordedSessionWithItems create: %v", err)
	}
	for i := range prices { // bounded by len(prices)
		in := billing.LineItemInput{Description: "work", Unit: "EA", Quantity: 1, UnitPrice: prices[i]}
		if _, err := repo.CreateItem(ctx, tenantID, sh.ID, in); err != nil {
			t.Fatalf("recordedSessionWithItems CreateItem: %v", err)
		}
	}
	return sh.ID
}

// newDraftHarness builds an invoice service wired to a real *session.Service over
// one shared connection, plus the session repo for seeding.
func newDraftHarness(t *testing.T) (*Service, *session.Service, *session.SessionsRepo, string, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	hub := realtime.NewHub()
	sessionSvc := session.NewService(conn, hub, NewInvoices(conn))
	invSvc := NewService(conn, hub, sessionSvc)
	return invSvc, sessionSvc, session.NewSessions(conn), tenantID, clientID
}

func TestDraftFromSessionsLinksItemsAndTotals(t *testing.T) {
	invSvc, sessionSvc, repo, tenantID, clientID := newDraftHarness(t)
	ctx := tctx(tenantID)

	s1 := recordedSessionWithItems(t, sessionSvc, repo, tenantID, clientID, "2026-01-10", 10, 20)
	s2 := recordedSessionWithItems(t, sessionSvc, repo, tenantID, clientID, "2026-01-11", 30, 40)

	inv, err := invSvc.DraftFromSessions(ctx, []string{s1, s2})
	if err != nil {
		t.Fatalf("DraftFromSessions: %v", err)
	}
	if inv == nil {
		t.Fatal("DraftFromSessions returned nil invoice")
	}
	if inv.ClientID != clientID {
		t.Fatalf("invoice client = %s, want %s", inv.ClientID, clientID)
	}
	if len(inv.LineItems) != 4 {
		t.Fatalf("invoice line items = %d, want 4", len(inv.LineItems))
	}
	for _, li := range inv.LineItems {
		if li.InvoiceID == nil || *li.InvoiceID != inv.ID {
			t.Fatalf("line item not linked to invoice: %+v", li)
		}
		if li.SessionID == nil {
			t.Fatalf("session item lost its session_id: %+v", li)
		}
	}
	if inv.Subtotal != 100 || inv.Total != 100 {
		t.Fatalf("invoice subtotal=%v total=%v, want 100/100", inv.Subtotal, inv.Total)
	}

	for _, sid := range []string{s1, s2} {
		sh, _ := sessionSvc.Get(ctx, sid)
		if sh == nil || sh.Status != "drafted" || sh.InvoiceID == nil || *sh.InvoiceID != inv.ID {
			t.Fatalf("session %s after draft = %+v, want drafted + invoice %s", sid, sh, inv.ID)
		}
	}
}

func TestDraftFromSessionsEmptySessionErrors(t *testing.T) {
	invSvc, sessionSvc, repo, tenantID, clientID := newDraftHarness(t)
	ctx := tctx(tenantID)

	withItems := recordedSessionWithItems(t, sessionSvc, repo, tenantID, clientID, "2026-01-10", 10)
	// A recorded session with zero items (G5).
	empty, err := sessionSvc.Create(ctx, session.SessionInput{ClientID: clientID, ServiceDate: "2026-01-11", Status: "recorded"})
	if err != nil {
		t.Fatalf("create empty session: %v", err)
	}

	if _, err := invSvc.DraftFromSessions(ctx, []string{withItems, empty.ID}); err == nil {
		t.Fatal("DraftFromSessions with an empty session must error (G5)")
	}
	// Nothing should have been linked: the non-empty session stays recorded.
	sh, _ := sessionSvc.Get(ctx, withItems)
	if sh == nil || sh.Status != "recorded" || sh.InvoiceID != nil {
		t.Fatalf("session must stay recorded after a failed draft, got %+v", sh)
	}
}

func TestDraftFromSessionsSingleClientGuard(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T2")
	p1 := seedClient(t, conn, tid, "P1")
	p2 := seedClient(t, conn, tid, "P2")
	hub := realtime.NewHub()
	shSvc := session.NewService(conn, hub, NewInvoices(conn))
	iSvc := NewService(conn, hub, shSvc)
	r := session.NewSessions(conn)

	a := recordedSessionWithItems(t, shSvc, r, tid, p1, "2026-01-10", 10)
	b := recordedSessionWithItems(t, shSvc, r, tid, p2, "2026-01-11", 20)

	if _, err := iSvc.DraftFromSessions(tctx(tid), []string{a, b}); err == nil {
		t.Fatal("DraftFromSessions across two clients must error")
	}

	// And a clean single-client pair still drafts.
	c := recordedSessionWithItems(t, shSvc, r, tid, p1, "2026-01-12", 5)
	if _, err := iSvc.DraftFromSessions(tctx(tid), []string{c}); err != nil {
		t.Fatalf("single-client DraftFromSessions: %v", err)
	}
}
