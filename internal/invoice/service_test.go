package invoice

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
)

func TestInvoiceCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID, clientID := newInvoiceSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	inv, err := svc.Create(ctx, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 2, UnitPrice: 10}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv == nil {
		t.Fatal("Create returned nil invoice")
	}
	select {
	case e := <-ch:
		if e.Entity != "invoice" || e.UUID != inv.ID || e.Action != "create" {
			t.Fatalf("event=%+v want invoice/%s/create", e, inv.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestInvoiceUpdateStatusBroadcasts(t *testing.T) {
	svc, hub, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	inv, err := svc.Create(ctx, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.UpdateStatus(ctx, inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "invoice" || e.UUID != inv.ID || e.Action != "status" {
			t.Fatalf("event=%+v want invoice/%s/status", e, inv.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after UpdateStatus")
	}
}

func TestInvoiceCreateEmptyItemsNoEvent(t *testing.T) {
	svc, hub, tenantID, clientID := newInvoiceSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, nil); err == nil {
		t.Fatal("empty items must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
