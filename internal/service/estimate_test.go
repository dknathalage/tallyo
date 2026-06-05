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

func newEstimateSvc(t *testing.T) (*EstimateService, *realtime.Hub, *repository.ClientsRepo) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "est.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewEstimateService(conn, hub), hub, repository.NewClients(conn)
}

func TestEstimateCreateBroadcasts(t *testing.T) {
	svc, hub, clients := newEstimateSvc(t)
	clientID := seedClient(t, clients)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := context.Background()

	est, err := svc.Create(ctx, repository.EstimateInput{
		ClientID: clientID, Date: "2026-01-01", ValidUntil: "2026-02-01",
	}, []repository.LineItemInput{{Description: "A", Quantity: 2, Rate: 10}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if est == nil {
		t.Fatal("Create returned nil estimate")
	}
	select {
	case e := <-ch:
		if e.Entity != "estimate" || e.ID != est.ID || e.Action != "create" {
			t.Fatalf("event=%+v want estimate/%d/create", e, est.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestEstimateConvertBroadcastsEstimateAndInvoice(t *testing.T) {
	svc, hub, clients := newEstimateSvc(t)
	clientID := seedClient(t, clients)
	ctx := context.Background()

	est, err := svc.Create(ctx, repository.EstimateInput{
		ClientID: clientID, Date: "2026-01-01", ValidUntil: "2026-02-01",
	}, []repository.LineItemInput{{Description: "A", Quantity: 1, Rate: 5}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.UpdateStatus(ctx, est.ID, "accepted"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	ch, unsub := hub.Subscribe()
	defer unsub()

	res, err := svc.Convert(ctx, est.ID)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res == nil {
		t.Fatal("Convert returned nil result")
	}

	var sawEstimate, sawInvoice bool
	deadline := time.After(time.Second)
	for i := 0; i < 2; i++ { // bounded: exactly two events expected
		select {
		case e := <-ch:
			if e.Entity == "estimate" && e.ID == est.ID && e.Action == "convert" {
				sawEstimate = true
			}
			if e.Entity == "invoice" && e.ID == res.InvoiceID && e.Action == "create" {
				sawInvoice = true
			}
		case <-deadline:
			t.Fatalf("timed out; sawEstimate=%v sawInvoice=%v", sawEstimate, sawInvoice)
		}
	}
	if !sawEstimate || !sawInvoice {
		t.Fatalf("missing event: sawEstimate=%v sawInvoice=%v", sawEstimate, sawInvoice)
	}
}
