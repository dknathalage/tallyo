package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newInvoiceSvc(t *testing.T) (*InvoiceService, *realtime.Hub, *repository.ClientsRepo) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "inv.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewInvoiceService(conn, hub), hub, repository.NewClients(conn)
}

// seedClient inserts a client so invoices have a valid FK.
func seedClient(t *testing.T, clients *repository.ClientsRepo) int64 {
	t.Helper()
	c, err := clients.Create(context.Background(), repository.ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	return c.ID
}

func TestInvoiceCreateBroadcasts(t *testing.T) {
	svc, hub, clients := newInvoiceSvc(t)
	clientID := seedClient(t, clients)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := context.Background()

	inv, err := svc.Create(ctx, repository.InvoiceInput{
		ClientID: clientID, Date: "2026-01-01", DueDate: "2026-02-01",
	}, []repository.LineItemInput{{Description: "A", Quantity: 2, Rate: 10}})
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
	svc, hub, clients := newInvoiceSvc(t)
	clientID := seedClient(t, clients)
	ctx := context.Background()

	inv, err := svc.Create(ctx, repository.InvoiceInput{
		ClientID: clientID, Date: "2026-01-01", DueDate: "2026-02-01",
	}, []repository.LineItemInput{{Description: "A", Quantity: 1, Rate: 5}})
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
	svc, hub, clients := newInvoiceSvc(t)
	clientID := seedClient(t, clients)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(context.Background(), repository.InvoiceInput{
		ClientID: clientID, Date: "2026-01-01", DueDate: "2026-02-01",
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
