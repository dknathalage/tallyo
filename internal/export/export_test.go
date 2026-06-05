package export

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/repository"
)

func sampleItems() []*repository.CatalogItem {
	return []*repository.CatalogItem{
		{ID: 1, Name: "Consulting", Rate: 150.5, Unit: "hour", Category: "Services", Sku: "CON-1"},
		{ID: 2, Name: "Design", Rate: 90, Unit: "hour", Category: "Services", Sku: "DES-2"},
	}
}

func sampleInvoices() []*repository.Invoice {
	return []*repository.Invoice{
		{InvoiceNumber: "INV-001", ClientName: "Acme", Date: "2026-01-01", DueDate: "2026-01-31", Status: "sent", Subtotal: 100, TaxAmount: 10, Total: 110, CurrencyCode: "USD"},
		{InvoiceNumber: "INV-002", ClientName: "Globex", Date: "2026-02-01", DueDate: "2026-02-28", Status: "paid", Subtotal: 200, TaxAmount: 20, Total: 220, CurrencyCode: "EUR"},
	}
}

func sampleEstimates() []*repository.Estimate {
	return []*repository.Estimate{
		{EstimateNumber: "EST-001", ClientName: "Acme", Date: "2026-01-01", ValidUntil: "2026-01-31", Status: "draft", Subtotal: 100, TaxAmount: 10, Total: 110, CurrencyCode: "USD"},
		{EstimateNumber: "EST-002", ClientName: "Globex", Date: "2026-02-01", ValidUntil: "2026-02-28", Status: "accepted", Subtotal: 200, TaxAmount: 20, Total: 220, CurrencyCode: "EUR"},
	}
}

func lines(b []byte) []string {
	s := strings.TrimRight(string(b), "\r\n")
	return strings.Split(s, "\n")
}

func TestCatalogCSV(t *testing.T) {
	b, err := CatalogCSV(sampleItems())
	if err != nil {
		t.Fatalf("CatalogCSV: %v", err)
	}
	got := lines(b)
	if len(got) != 3 {
		t.Fatalf("want 3 lines (header + 2), got %d: %q", len(got), got)
	}
	if strings.TrimRight(got[0], "\r") != "name,sku,rate,unit,category" {
		t.Fatalf("header mismatch: %q", got[0])
	}
	if !strings.Contains(got[1], "Consulting") || !strings.Contains(got[1], "150.50") {
		t.Fatalf("row 1 mismatch: %q", got[1])
	}
	if !strings.Contains(got[2], "90.00") {
		t.Fatalf("row 2 rate not formatted: %q", got[2])
	}
}

func TestInvoicesCSV(t *testing.T) {
	b, err := InvoicesCSV(sampleInvoices())
	if err != nil {
		t.Fatalf("InvoicesCSV: %v", err)
	}
	got := lines(b)
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d: %q", len(got), got)
	}
	if strings.TrimRight(got[0], "\r") != "invoiceNumber,clientName,date,dueDate,status,subtotal,taxAmount,total,currency" {
		t.Fatalf("header mismatch: %q", got[0])
	}
	if !strings.Contains(got[1], "INV-001") || !strings.Contains(got[1], "110.00") {
		t.Fatalf("row 1 mismatch: %q", got[1])
	}
}

func TestEstimatesCSV(t *testing.T) {
	b, err := EstimatesCSV(sampleEstimates())
	if err != nil {
		t.Fatalf("EstimatesCSV: %v", err)
	}
	got := lines(b)
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d: %q", len(got), got)
	}
	if strings.TrimRight(got[0], "\r") != "estimateNumber,clientName,date,validUntil,status,subtotal,taxAmount,total,currency" {
		t.Fatalf("header mismatch: %q", got[0])
	}
	if !strings.Contains(got[1], "EST-001") {
		t.Fatalf("row 1 mismatch: %q", got[1])
	}
}

func TestCatalogXLSX(t *testing.T) {
	b, err := CatalogXLSX(sampleItems())
	if err != nil {
		t.Fatalf("CatalogXLSX: %v", err)
	}
	if len(b) <= 500 {
		t.Fatalf("xlsx too small: %d bytes", len(b))
	}
	if !bytes.HasPrefix(b, []byte("PK")) {
		t.Fatalf("xlsx must be a zip starting with PK, got %q", b[:2])
	}
}
