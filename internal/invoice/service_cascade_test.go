package invoice

import (
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/session"
)

// seedDraftedSession creates a recorded session and drafts it onto inv via the
// session.Service, returning the session id.
func seedDraftedSession(t *testing.T, sessionSvc *session.Service, tenantID, clientID, invoiceID string) string {
	t.Helper()
	ctx := tctx(tenantID)
	sh, err := sessionSvc.Create(ctx, session.SessionInput{ClientID: clientID, ServiceDate: "2026-01-15"})
	if err != nil {
		t.Fatalf("seedDraftedSession create: %v", err)
	}
	if err := sessionSvc.MarkDrafted(ctx, invoiceID, []string{sh.ID}); err != nil {
		t.Fatalf("seedDraftedSession MarkDrafted: %v", err)
	}
	return sh.ID
}

func TestInvoiceStatusCascadesToSessions(t *testing.T) {
	for _, status := range []string{"sent", "paid"} {
		t.Run(status, func(t *testing.T) {
			conn := newTestDB(t)
			tenantID := seedTenant(t, conn, "Acme")
			clientID := seedClient(t, conn, tenantID, "Jane Client")
			hub := realtime.NewHub()
			invSvc := NewService(conn, hub, session.NewService(conn, hub, NewInvoices(conn)))
			sessionSvc := session.NewService(conn, hub, NewInvoices(conn))
			ctx := tctx(tenantID)

			inv, err := invSvc.Create(ctx, InvoiceInput{
				ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
			}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
			if err != nil {
				t.Fatalf("Create invoice: %v", err)
			}
			sessionID := seedDraftedSession(t, sessionSvc, tenantID, clientID, inv.ID)

			if err := invSvc.UpdateStatus(ctx, inv.ID, status); err != nil {
				t.Fatalf("UpdateStatus %s: %v", status, err)
			}
			got, _ := sessionSvc.Get(ctx, sessionID)
			if got == nil || got.Status != status {
				t.Fatalf("session status after invoice %s = %+v, want %s", status, got, status)
			}
		})
	}
}

func TestInvoiceStatusDoesNotCascadeForDraft(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	hub := realtime.NewHub()
	invSvc := NewService(conn, hub, session.NewService(conn, hub, NewInvoices(conn)))
	sessionSvc := session.NewService(conn, hub, NewInvoices(conn))
	ctx := tctx(tenantID)

	inv, err := invSvc.Create(ctx, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create invoice: %v", err)
	}
	sessionID := seedDraftedSession(t, sessionSvc, tenantID, clientID, inv.ID)

	// A non-terminal status (e.g. back to draft) must NOT advance the session.
	if err := invSvc.UpdateStatus(ctx, inv.ID, "draft"); err != nil {
		t.Fatalf("UpdateStatus draft: %v", err)
	}
	got, _ := sessionSvc.Get(ctx, sessionID)
	if got == nil || got.Status != "drafted" {
		t.Fatalf("session should stay drafted on invoice draft status, got %+v", got)
	}
}

func TestInvoiceDeleteRevertsSessionsToRecorded(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	hub := realtime.NewHub()
	invSvc := NewService(conn, hub, session.NewService(conn, hub, NewInvoices(conn)))
	sessionSvc := session.NewService(conn, hub, NewInvoices(conn))
	ctx := tctx(tenantID)

	inv, err := invSvc.Create(ctx, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create invoice: %v", err)
	}
	sessionID := seedDraftedSession(t, sessionSvc, tenantID, clientID, inv.ID)

	if err := invSvc.Delete(ctx, inv.ID); err != nil {
		t.Fatalf("Delete invoice: %v", err)
	}
	got, _ := sessionSvc.Get(ctx, sessionID)
	if got == nil || got.Status != "recorded" || got.InvoiceID != nil {
		t.Fatalf("session after invoice delete = %+v, want recorded + nil invoice", got)
	}
}
