package service

import (
	"context"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
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
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	inv, err := svc.Create(ctx, repository.InvoiceInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 2, UnitPrice: 10}})
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
		if e.Entity != "invoice" || e.ID != inv.ID || e.Action != "status" {
			t.Fatalf("event=%+v want invoice/%d/status", e, inv.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after UpdateStatus")
	}
}

// TestSweepSkipsSuspendedAndScopesBroadcast covers the per-tenant sweep (spec
// §8): ActiveTenantIDs excludes a suspended tenant, and MarkOverdueForTenant
// flips only the swept tenant's invoices and broadcasts ONLY to that tenant's
// subscribers (not a sibling tenant's).
func TestSweepSkipsSuspendedAndScopesBroadcast(t *testing.T) {
	conn := newTestDB(t)
	hub := realtime.NewHub()
	svc := NewInvoiceService(conn, hub)

	tenantA := seedTenant(t, conn)          // active
	tenantB := seedSuspendedTenant(t, conn) // suspended
	partA := seedParticipant(t, conn, tenantA)
	partB := seedParticipant(t, conn, tenantB)

	// One sent, past-due invoice per tenant.
	overdueA := seedSentPastDue(t, conn, svc, tenantA, partA)
	seedSentPastDue(t, conn, svc, tenantB, partB)

	// ActiveTenantIDs must exclude the suspended tenant B.
	ids, err := svc.ActiveTenantIDs(context.Background())
	if err != nil {
		t.Fatalf("ActiveTenantIDs: %v", err)
	}
	if containsID(ids, tenantB) {
		t.Fatalf("suspended tenant %d must not appear in active ids %v", tenantB, ids)
	}
	if !containsID(ids, tenantA) {
		t.Fatalf("active tenant %d missing from active ids %v", tenantA, ids)
	}

	// Subscribe both tenants; only A should receive the overdue sweep event.
	chA, unsubA := hub.Subscribe(tenantA)
	defer unsubA()
	chB, unsubB := hub.Subscribe(tenantB)
	defer unsubB()

	rows, err := svc.MarkOverdueForTenant(tctx(tenantA), tenantA)
	if err != nil {
		t.Fatalf("MarkOverdueForTenant: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != overdueA.ID {
		t.Fatalf("swept rows = %+v, want the one tenant-A invoice %d", rows, overdueA.ID)
	}

	select {
	case e := <-chA:
		if e.Action != "overdue_sweep" || e.TenantID != tenantA {
			t.Fatalf("tenant A event = %+v, want overdue_sweep for tenant %d", e, tenantA)
		}
	case <-time.After(time.Second):
		t.Fatal("tenant A did not receive overdue_sweep")
	}
	select {
	case e := <-chB:
		t.Fatalf("tenant B leaked sweep event %+v", e)
	case <-time.After(150 * time.Millisecond):
		// expected: nothing for B
	}
}

func TestInvoiceCreateEmptyItemsNoEvent(t *testing.T) {
	svc, hub, tenantID, participantID := newInvoiceSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
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
