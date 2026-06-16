package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newEstimateSvc(t *testing.T) (*EstimateService, *realtime.Hub, int64, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	hub := realtime.NewHub()
	return NewEstimateService(conn, hub), hub, tenantID, participantID
}

func TestEstimateCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantID := newEstimateSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := tctx(tenantID)

	est, err := svc.Create(ctx, repository.EstimateInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", ValidUntil: "2026-02-01",
	}, []repository.LineItemInput{{Description: "A", Quantity: 2, UnitPrice: 10}})
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
	svc, hub, tenantID, participantID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est, err := svc.Create(ctx, repository.EstimateInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", ValidUntil: "2026-02-01",
	}, []repository.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
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

func TestEstimateDuplicateBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est, err := svc.Create(ctx, repository.EstimateInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", ValidUntil: "2026-02-01",
	}, []repository.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe()
	defer unsub()

	dup, err := svc.Duplicate(ctx, est.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup == nil {
		t.Fatal("Duplicate returned nil estimate")
	}
	select {
	case e := <-ch:
		if e.Entity != "estimate" || e.ID != dup.ID || e.Action != "create" {
			t.Fatalf("event=%+v want estimate/%d/create", e, dup.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Duplicate")
	}
}
