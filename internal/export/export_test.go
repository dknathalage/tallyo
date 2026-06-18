package export

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/customitem"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/repository"
)

func sampleItems() []*customitem.CustomItem {
	return []*customitem.CustomItem{
		{ID: 1, Name: "Consulting", Rate: 150.5, Unit: "hour"},
		{ID: 2, Name: "Design", Rate: 90, Unit: "hour", GstFree: true},
	}
}

func sampleInvoices() []*invoice.Invoice {
	return []*invoice.Invoice{
		{Number: "INV-001", ParticipantName: "Acme", IssueDate: "2026-01-01", DueDate: "2026-01-31", Status: "sent", Subtotal: 100, Tax: 10, Total: 110},
		{Number: "INV-002", ParticipantName: "Globex", IssueDate: "2026-02-01", DueDate: "2026-02-28", Status: "paid", Subtotal: 200, Tax: 20, Total: 220},
	}
}

func sampleEstimates() []*repository.Estimate {
	return []*repository.Estimate{
		{Number: "EST-001", ParticipantName: "Acme", IssueDate: "2026-01-01", ValidUntil: "2026-01-31", Status: "draft", Subtotal: 100, Tax: 10, Total: 110},
		{Number: "EST-002", ParticipantName: "Globex", IssueDate: "2026-02-01", ValidUntil: "2026-02-28", Status: "accepted", Subtotal: 200, Tax: 20, Total: 220},
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
	if strings.TrimRight(got[0], "\r") != "name,rate,unit,gstFree" {
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
	if strings.TrimRight(got[0], "\r") != "number,participantName,issueDate,dueDate,status,subtotal,tax,total" {
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
	if strings.TrimRight(got[0], "\r") != "number,participantName,issueDate,validUntil,status,subtotal,tax,total" {
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
