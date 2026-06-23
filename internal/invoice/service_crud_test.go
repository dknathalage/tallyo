package invoice

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/session"
)

func TestInvoiceListAndGet(t *testing.T) {
	svc, _, tenantID, clientID := newInvoiceSvc(t)
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
		t.Fatalf("List[0].ID = %d, want %d", list[0].ID, inv.ID)
	}

	got, err := svc.Get(ctx, inv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ID != inv.ID {
		t.Fatalf("Get = %+v, want id %d", got, inv.ID)
	}
}

func TestInvoiceGetNotFoundReturnsNil(t *testing.T) {
	svc, _, tenantID, _ := newInvoiceSvc(t)

	got, err := svc.Get(tctx(tenantID), 999999)
	if err != nil {
		t.Fatalf("Get missing: unexpected err %v", err)
	}
	if got != nil {
		t.Fatalf("Get missing = %+v, want nil", got)
	}
}

func TestInvoiceListByStatusSvc(t *testing.T) {
	svc, _, tenantID, clientID := newInvoiceSvc(t)
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
		t.Fatalf("ListByStatus sent = %+v, want one id %d", sent, inv.ID)
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
	hub := realtime.NewHub()
	svc := NewService(conn, hub, session.NewService(conn, hub, NewInvoices(conn)))
	ctx := tctx(tenantID)

	inv := makeInvoice(t, svc, tenantID, clientID)

	rows, err := svc.ListClientInvoices(ctx, clientID)
	if err != nil {
		t.Fatalf("ListClientInvoices: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != inv.ID {
		t.Fatalf("ListClientInvoices = %+v, want one id %d", rows, inv.ID)
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
	svc, _, tenantID, clientID := newInvoiceSvc(t)
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
	svc, _, tenantID, clientID := newInvoiceSvc(t)

	got, err := svc.Update(tctx(tenantID), 999999, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-03-01", DueDate: "2026-04-01",
	}, []billing.LineItemInput{{Description: "B", Quantity: 1, UnitPrice: 1}})
	if err != nil {
		t.Fatalf("Update missing: unexpected err %v", err)
	}
	if got != nil {
		t.Fatalf("Update missing = %+v, want nil", got)
	}
}

func TestInvoiceDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	inv := makeInvoice(t, svc, tenantID, clientID)

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.Delete(ctx, inv.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "invoice" || e.UUID != inv.UUID || e.Action != "delete" {
			t.Fatalf("event=%+v want invoice/%d/delete", e, inv.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Delete")
	}

	got, err := svc.Get(ctx, inv.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("invoice %d still present after delete", inv.ID)
	}
}

func TestInvoiceBulkDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	a := makeInvoice(t, svc, tenantID, clientID)
	b := makeInvoice(t, svc, tenantID, clientID)

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.BulkDelete(ctx, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "invoice" || e.UUID != "" || e.Action != "bulk_delete" {
			t.Fatalf("event=%+v want invoice/0/bulk_delete", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after BulkDelete")
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List after bulk delete = %d, want 0", len(list))
	}
}

func TestInvoiceBulkUpdateStatusBroadcasts(t *testing.T) {
	svc, hub, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	a := makeInvoice(t, svc, tenantID, clientID)
	b := makeInvoice(t, svc, tenantID, clientID)

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.BulkUpdateStatus(ctx, []int64{a.ID, b.ID}, "sent"); err != nil {
		t.Fatalf("BulkUpdateStatus: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "invoice" || e.UUID != "" || e.Action != "bulk_status" {
			t.Fatalf("event=%+v want invoice/0/bulk_status", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after BulkUpdateStatus")
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
	hub := realtime.NewHub()
	svc := NewService(conn, hub, session.NewService(conn, hub, NewInvoices(conn)))

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
		t.Fatalf("tenant B fetched tenant A invoice %d", inv.ID)
	}
}
