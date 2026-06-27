package invoice

import (
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
)

func TestInvoiceCreate(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)
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
}

func TestInvoiceUpdateStatus(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)
	ctx := tctx(tenantID)

	inv, err := svc.Create(ctx, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.UpdateStatus(ctx, inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
}

func TestInvoiceCreateEmptyItemsErrors(t *testing.T) {
	svc, tenantID, clientID := newInvoiceSvc(t)

	if _, err := svc.Create(tctx(tenantID), InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, nil); err == nil {
		t.Fatal("empty items must error")
	}
}
