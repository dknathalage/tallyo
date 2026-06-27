package estimate

import (
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
)

func TestEstimateCreate(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
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
}

func TestEstimateConvert(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
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

	res, err := svc.Convert(ctx, est.ID)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res == nil {
		t.Fatal("Convert returned nil result")
	}
	if res.InvoiceUUID == "" {
		t.Fatal("Convert returned empty invoice uuid")
	}
}

func TestEstimateDuplicateViaService(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est, err := svc.Create(ctx, EstimateInput{
		ClientID: clientID, IssueDate: "2026-01-01", ValidUntil: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dup, err := svc.Duplicate(ctx, est.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup == nil {
		t.Fatal("Duplicate returned nil estimate")
	}
}
