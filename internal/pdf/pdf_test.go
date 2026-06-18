package pdf

import (
	"bytes"
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
)

func TestRenderInvoiceProducesPDF(t *testing.T) {
	inv := &InvoiceDoc{
		Number: "INV-0001", IssueDate: "2026-06-05", DueDate: "2026-07-05",
		BusinessSnapshot: `{"name":"Acme LLC","email":"acme@x.com","address":"1 St"}`,
		ClientSnapshot:   `{"name":"Client Co","email":"c@x.com"}`,
		Subtotal:         25, Tax: 2.5, Total: 27.5,
		Status: "draft", Notes: "thanks",
		LineItems: []*billing.LineItem{
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
	est := &EstimateDoc{
		Number: "EST-0001", IssueDate: "2026-06-05", ValidUntil: "2026-07-05",
		BusinessSnapshot: `{"name":"Acme LLC"}`, ClientSnapshot: `{"name":"Client Co"}`,
		Subtotal: 25, Tax: 2.5, Total: 27.5, Status: "draft",
		LineItems: []*billing.LineItem{{Description: "Widget", Quantity: 2, UnitPrice: 10, LineTotal: 20}},
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

// TestMoneyConsistentCurrency locks the single currency presentation used across
// the whole document: line-item amounts and the totals block must all render via
// money() so they share one "AUD <amount>" format.
func TestMoneyConsistentCurrency(t *testing.T) {
	cases := map[float64]string{
		10:   "AUD 10.00",
		2.5:  "AUD 2.50",
		27.5: "AUD 27.50",
		0:    "AUD 0.00",
	}
	for v, want := range cases {
		if got := money(v); got != want {
			t.Fatalf("money(%g)=%q want %q", v, got, want)
		}
	}
}
