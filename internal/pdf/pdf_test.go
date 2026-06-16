package pdf

import (
	"bytes"
	"testing"

	"github.com/dknathalage/tallyo/internal/repository"
)

func TestRenderInvoiceProducesPDF(t *testing.T) {
	inv := &repository.Invoice{
		Number: "INV-0001", IssueDate: "2026-06-05", DueDate: "2026-07-05",
		BusinessSnapshot: `{"name":"Acme LLC","email":"acme@x.com","address":"1 St"}`,
		ClientSnapshot:   `{"name":"Client Co","email":"c@x.com"}`,
		Subtotal:         25, Tax: 2.5, Total: 27.5,
		Status: "draft", Notes: "thanks",
		LineItems: []*repository.LineItem{
			{Description: "Widget", Quantity: 2, UnitPrice: 10, LineTotal: 20},
			{Description: "Gadget", Quantity: 1, UnitPrice: 5, LineTotal: 5},
		},
	}
	b, err := RenderInvoice(inv)
	if err != nil {
		t.Fatalf("RenderInvoice: %v", err)
	}
	if len(b) < 500 {
		t.Fatalf("pdf too small: %d", len(b))
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("not a PDF: %q", b[:8])
	}
}

func TestRenderEstimateProducesPDF(t *testing.T) {
	est := &repository.Estimate{
		Number: "EST-0001", IssueDate: "2026-06-05", ValidUntil: "2026-07-05",
		BusinessSnapshot: `{"name":"Acme LLC"}`, ClientSnapshot: `{"name":"Client Co"}`,
		Subtotal: 25, Tax: 2.5, Total: 27.5, Status: "draft",
		LineItems: []*repository.LineItem{{Description: "Widget", Quantity: 2, UnitPrice: 10, LineTotal: 20}},
	}
	b, err := RenderEstimate(est)
	if err != nil {
		t.Fatalf("RenderEstimate: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("not a PDF")
	}
}

func TestRenderNilErrors(t *testing.T) {
	if _, err := RenderInvoice(nil); err == nil {
		t.Fatal("nil invoice must error")
	}
}
