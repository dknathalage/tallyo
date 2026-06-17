package agent

// Deterministic tests for create_invoice keyed on recorded SHIFTS
// (NewCreateInvoiceToolForShifts). They mirror the notes-path verify tests but
// over the shifts lifecycle: catalogue-authoritative pricing, the completeness
// verify (a missing shift or a custom-substituted line is rejected with no orphan
// invoice), billing marks the covered shifts drafted+linked, and a dropped day is
// caught via the from/to range.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

// verifyLine is the create_invoice line shape used by the completeness tests.
type verifyLine struct {
	Code        string  `json:"code,omitempty"`
	Description string  `json:"description,omitempty"`
	ServiceDate string  `json:"serviceDate,omitempty"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice,omitempty"`
}

// fullCodedLines is the correct 8-line draft (transport + self-care per day).
func fullCodedLines() []verifyLine {
	out := make([]verifyLine, 0, 2*len(referenceWeek))
	for i := range referenceWeek {
		d := referenceWeek[i]
		out = append(out,
			verifyLine{Code: codeTransport, ServiceDate: d.date, Quantity: d.km},
			verifyLine{Code: codeSelfCare, ServiceDate: d.date, Quantity: d.hr},
		)
	}
	return out
}

// assertNoInvoice fails the test when any invoice has been persisted — a rejected
// draft must leave no orphan.
func assertNoInvoice(t *testing.T, inv *service.InvoiceService, ctx context.Context) {
	t.Helper()
	rows, err := inv.List(ctx)
	if err != nil {
		t.Fatalf("list invoices: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a rejected draft must not persist an invoice; found %d", len(rows))
	}
}

// shiftsCreateFixture seeds the nursing-note fixture as recorded shifts (tenant,
// Tania, catalogue, 4 shifts) and returns the shift-keyed create_invoice tool,
// the invoice and shift services, and an authed context.
func shiftsCreateFixture(t *testing.T) (Tool, *service.InvoiceService, *service.ShiftService, context.Context, int64) {
	t.Helper()
	conn, tenantID, participantID := shiftToolsFixture(t)
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	shifts := service.NewShiftService(conn, realtime.NewHub())
	seedReferenceShifts(t, shifts, ctx, participantID)
	inv := service.NewInvoiceService(conn, realtime.NewHub())
	tool := NewCreateInvoiceToolForShifts(inv, shifts, nil)
	return tool, inv, shifts, ctx, participantID
}

// runShiftCreate invokes the tool with the given lines over the full reference
// range, declaring from/to so a dropped day is detected.
func runShiftCreate(t *testing.T, tool Tool, ctx context.Context, participantID int64, items []verifyLine, from, to string) (Result, error) {
	t.Helper()
	in := map[string]any{
		"participantId": participantID, "issueDate": "2026-06-14",
		"from": from, "to": to, "items": items,
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return tool.Handler(ctx, raw)
}

// TestShiftCreateFullyCodedDraftSucceeds asserts a complete, catalogue-coded
// draft is priced authoritatively and persists at the reference total.
func TestShiftCreateFullyCodedDraftSucceeds(t *testing.T) {
	tool, _, _, ctx, pid := shiftsCreateFixture(t)
	res, err := runShiftCreate(t, tool, ctx, pid, fullCodedLines(), "2026-06-09", "2026-06-12")
	if err != nil {
		t.Fatalf("create_invoice: %v", err)
	}
	got, ok := res.JSON.(*repository.Invoice)
	if !ok {
		t.Fatalf("result JSON is %T, want *repository.Invoice", res.JSON)
	}
	if got.Total != 1905.76 {
		t.Fatalf("total = %.2f, want 1905.76", got.Total)
	}
	if got.Tax != 0 {
		t.Fatalf("tax = %.2f, want 0 (NDIS supports are GST-free)", got.Tax)
	}
}

// TestShiftCreateMissingSelfCareRejected asserts that omitting the self-care
// lines (every support hour unbilled) is rejected with no orphan invoice.
func TestShiftCreateMissingSelfCareRejected(t *testing.T) {
	tool, inv, _, ctx, pid := shiftsCreateFixture(t)
	var items []verifyLine
	for i := range referenceWeek {
		items = append(items, verifyLine{Code: codeTransport, ServiceDate: referenceWeek[i].date, Quantity: referenceWeek[i].km})
	}
	_, err := runShiftCreate(t, tool, ctx, pid, items, "2026-06-09", "2026-06-12")
	if err == nil {
		t.Fatal("expected a completeness error for the omitted self-care lines")
	}
	if !strings.Contains(err.Error(), "hours") {
		t.Fatalf("error %q should name the missing support hours", err.Error())
	}
	assertNoInvoice(t, inv, ctx)
}

// TestShiftCreateCustomLineInsteadOfCodeRejected asserts a support billed as a
// custom (uncoded) line is rejected — right quantity, wrong shape.
func TestShiftCreateCustomLineInsteadOfCodeRejected(t *testing.T) {
	tool, inv, _, ctx, pid := shiftsCreateFixture(t)
	var items []verifyLine
	for i := range referenceWeek {
		d := referenceWeek[i]
		items = append(items,
			verifyLine{Code: codeTransport, ServiceDate: d.date, Quantity: d.km},
			verifyLine{Description: "self care", ServiceDate: d.date, Quantity: d.hr, UnitPrice: 65.0},
		)
	}
	_, err := runShiftCreate(t, tool, ctx, pid, items, "2026-06-09", "2026-06-12")
	if err == nil {
		t.Fatal("expected a completeness error: self-care billed as a custom line, not a code")
	}
	assertNoInvoice(t, inv, ctx)
}

// TestShiftCreateDroppedDayRejected asserts that billing only some days while
// declaring the full from/to range catches the dropped day (its supports lie
// outside the coded-line range).
func TestShiftCreateDroppedDayRejected(t *testing.T) {
	tool, inv, _, ctx, pid := shiftsCreateFixture(t)
	var items []verifyLine
	for i := 0; i < 3; i++ {
		d := referenceWeek[i]
		items = append(items,
			verifyLine{Code: codeTransport, ServiceDate: d.date, Quantity: d.km},
			verifyLine{Code: codeSelfCare, ServiceDate: d.date, Quantity: d.hr},
		)
	}
	_, err := runShiftCreate(t, tool, ctx, pid, items, "2026-06-09", "2026-06-12")
	if err == nil {
		t.Fatal("expected rejection: 2026-06-12 was dropped entirely")
	}
	assertNoInvoice(t, inv, ctx)
}

// TestShiftCreateMarksShiftsDrafted asserts a successful create links every
// covered shift to the new invoice and advances it to 'drafted'.
func TestShiftCreateMarksShiftsDrafted(t *testing.T) {
	tool, _, shifts, ctx, pid := shiftsCreateFixture(t)
	res, err := runShiftCreate(t, tool, ctx, pid, fullCodedLines(), "2026-06-09", "2026-06-12")
	if err != nil {
		t.Fatalf("create_invoice: %v", err)
	}
	created, ok := res.JSON.(*repository.Invoice)
	if !ok {
		t.Fatalf("result JSON is %T, want *repository.Invoice", res.JSON)
	}
	recs, err := shifts.ListParticipant(ctx, pid, "2026-06-09", "2026-06-12")
	if err != nil {
		t.Fatalf("list shifts: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("no shifts seeded")
	}
	for i := range recs { // bounded by len(recs)
		if recs[i].InvoiceID == nil || *recs[i].InvoiceID != created.ID {
			t.Fatalf("shift %s not linked to invoice %d (invoiceId=%v)", recs[i].ServiceDate, created.ID, recs[i].InvoiceID)
		}
		if recs[i].Status != "drafted" {
			t.Fatalf("shift %s status = %q, want drafted", recs[i].ServiceDate, recs[i].Status)
		}
	}
}
