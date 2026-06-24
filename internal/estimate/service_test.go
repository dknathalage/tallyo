package estimate

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
)

func TestEstimateCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID, clientID := newEstimateSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	est, err := svc.Create(ctx, EstimateInput{
		ClientID: clientID, IssueDate: "2026-01-01", ValidUntil: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 2, UnitPrice: 10}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if est == nil {
		t.Fatal("Create returned nil estimate")
	}
	select {
	case e := <-ch:
		if e.Entity != "estimate" || e.UUID != est.ID || e.Action != "create" {
			t.Fatalf("event=%+v want estimate/%s/create", e, est.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestEstimateConvertBroadcastsEstimateAndInvoice(t *testing.T) {
	svc, hub, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est, err := svc.Create(ctx, EstimateInput{
		ClientID: clientID, IssueDate: "2026-01-01", ValidUntil: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.UpdateStatus(ctx, est.ID, "accepted"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
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
			if e.Entity == "estimate" && e.UUID == est.ID && e.Action == "convert" {
				sawEstimate = true
			}
			if e.Entity == "invoice" && e.UUID == res.InvoiceUUID && e.Action == "create" {
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
	svc, hub, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est, err := svc.Create(ctx, EstimateInput{
		ClientID: clientID, IssueDate: "2026-01-01", ValidUntil: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
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
		if e.Entity != "estimate" || e.UUID != dup.ID || e.Action != "create" {
			t.Fatalf("event=%+v want estimate/%s/create", e, dup.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Duplicate")
	}
}
