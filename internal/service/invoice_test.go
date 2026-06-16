package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newInvoiceSvc(t *testing.T) (*InvoiceService, *realtime.Hub, int64, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	hub := realtime.NewHub()
	return NewInvoiceService(conn, hub), hub, tenantID, participantID
}

func TestInvoiceCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantID := newInvoiceSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := tctx(tenantID)

	inv, err := svc.Create(ctx, repository.InvoiceInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []repository.LineItemInput{{Description: "A", Quantity: 2, UnitPrice: 10}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv == nil {
		t.Fatal("Create returned nil invoice")
	}
	select {
	case e := <-ch:
		if e.Entity != "invoice" || e.ID != inv.ID || e.Action != "create" {
			t.Fatalf("event=%+v want invoice/%d/create", e, inv.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestInvoiceUpdateStatusBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	inv, err := svc.Create(ctx, repository.InvoiceInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []repository.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe()
	defer unsub()

	if err := svc.UpdateStatus(ctx, inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "invoice" || e.ID != inv.ID || e.Action != "status" {
			t.Fatalf("event=%+v want invoice/%d/status", e, inv.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after UpdateStatus")
	}
}

func TestInvoiceCreateEmptyItemsNoEvent(t *testing.T) {
	svc, hub, tenantID, participantID := newInvoiceSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), repository.InvoiceInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
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
