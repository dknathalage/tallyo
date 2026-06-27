package invoice

import (
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/session"
)

func TestInvoiceListAndGet(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	inv := makeInvoice(t, svc, tenantID, clientID)

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}
	if list[0].ID != inv.ID {
		t.Fatalf("List[0].ID = %s, want %s", list[0].ID, inv.ID)
	}

	got, err := svc.Get(ctx, inv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ID != inv.ID {
		t.Fatalf("Get = %+v, want id %s", got, inv.ID)
	}
}

func TestInvoiceGetNotFoundReturnsNil(t *testing.T) {
	svc, tenantID, _ := newInvoiceSvc(t)

	got, err := svc.Get(tctx(tenantID), "nonexistent-uuid")
	if err != nil {
		t.Fatalf("Get missing: unexpected err %v", err)
	}
	if got != nil {
		t.Fatalf("Get missing = %+v, want nil", got)
	}
}

func TestInvoiceListByStatusSvc(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	inv := makeInvoice(t, svc, tenantID, clientID)
	if err := svc.UpdateStatus(ctx, inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	sent, err := svc.ListByStatus(ctx, "sent")
	if err != nil {
		t.Fatalf("ListByStatus sent: %v", err)
	}
	if len(sent) != 1 || sent[0].ID != inv.ID {
		t.Fatalf("ListByStatus sent = %+v, want one id %s", sent, inv.ID)
	}

	draft, err := svc.ListByStatus(ctx, "draft")
	if err != nil {
		t.Fatalf("ListByStatus draft: %v", err)
	}
	if len(draft) != 0 {
		t.Fatalf("ListByStatus draft = %d, want 0", len(draft))
	}
}

func TestInvoiceListClientInvoicesAndStats(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID, clientUUID := seedClientUUID(t, conn, tenantID, "Jane Client")
	svc := NewService(conn, session.NewService(conn, NewInvoices(conn)))
	ctx := tctx(tenantID)

	inv := makeInvoice(t, svc, tenantID, clientID)

	rows, err := svc.ListClientInvoices(ctx, clientID)
	if err != nil {
		t.Fatalf("ListClientInvoices: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != inv.ID {
		t.Fatalf("ListClientInvoices = %+v, want one id %s", rows, inv.ID)
	}

	// ClientStats now resolves the client uuid → int PK.
	stats, err := svc.ClientStats(ctx, clientUUID)
	if err != nil {
		t.Fatalf("ClientStats: %v", err)
	}
	if stats == nil {
		t.Fatal("ClientStats returned nil")
	}

	// An unknown client uuid yields no stats (handler 404s).
	none, err := svc.ClientStats(ctx, "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c")
	if err != nil {
		t.Fatalf("ClientStats unknown: %v", err)
	}
	if none != nil {
		t.Fatalf("ClientStats unknown = %+v, want nil", none)
	}
}

func TestInvoiceUpdate(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	inv := makeInvoice(t, svc, tenantID, clientID)

	updated, err := svc.Update(ctx, inv.ID, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-03-01", DueDate: "2026-04-01",
	}, []billing.LineItemInput{{Description: "B", Quantity: 3, UnitPrice: 7}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("Update returned nil")
	}
	if updated.IssueDate != "2026-03-01" {
		t.Fatalf("Update IssueDate = %q, want 2026-03-01", updated.IssueDate)
	}
}

func TestInvoiceUpdateNotFoundReturnsNil(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)

	got, err := svc.Update(tctx(tenantID), "nonexistent-uuid", InvoiceInput{
		ClientID: clientID, IssueDate: "2026-03-01", DueDate: "2026-04-01",
	}, []billing.LineItemInput{{Description: "B", Quantity: 1, UnitPrice: 1}})
	if err != nil {
		t.Fatalf("Update missing: unexpected err %v", err)
	}
	if got != nil {
		t.Fatalf("Update missing = %+v, want nil", got)
	}
}

func TestInvoiceDelete(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	inv := makeInvoice(t, svc, tenantID, clientID)

	if err := svc.Delete(ctx, inv.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := svc.Get(ctx, inv.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("invoice %s still present after delete", inv.ID)
	}
}

func TestInvoiceBulkDelete(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	a := makeInvoice(t, svc, tenantID, clientID)
	b := makeInvoice(t, svc, tenantID, clientID)

	if err := svc.BulkDelete(ctx, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List after bulk delete = %d, want 0", len(list))
	}
}

func TestInvoiceBulkUpdateStatus(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	a := makeInvoice(t, svc, tenantID, clientID)
	b := makeInvoice(t, svc, tenantID, clientID)

	if err := svc.BulkUpdateStatus(ctx, []string{a.ID, b.ID}, "sent"); err != nil {
		t.Fatalf("BulkUpdateStatus: %v", err)
	}

	sent, err := svc.ListByStatus(ctx, "sent")
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("sent invoices = %d, want 2", len(sent))
	}
}

// TestInvoiceTenantScoping asserts a second tenant cannot see or fetch the first
// tenant's invoice.
func TestInvoiceTenantScoping(t *testing.T) {
	conn := newTestDB(t)
	svc := NewService(conn, session.NewService(conn, NewInvoices(conn)))

	tenantA := seedTenant(t, conn, "Acme")
	partA := seedClient(t, conn, tenantA, "Jane")
	tenantB := seedTenant(t, conn, "Beta")

	inv := makeInvoice(t, svc, tenantA, partA)

	listB, err := svc.List(tctx(tenantB))
	if err != nil {
		t.Fatalf("List B: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("tenant B sees %d invoices, want 0", len(listB))
	}

	gotB, err := svc.Get(tctx(tenantB), inv.ID)
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}
	if gotB != nil {
		t.Fatalf("tenant B fetched tenant A invoice %s", inv.ID)
	}
}
