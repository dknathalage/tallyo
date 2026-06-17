package agent

// Pillar 4 — deterministic tests for the notes-completeness guard on the
// verified create_invoice tool. They prove the tool, BEFORE persisting, rejects
// a draft that doesn't cover every recorded support (a missing line, or a
// support billed as a custom line instead of an NDIS code) and leaves no orphan
// invoice; and that a fully-coded draft succeeds.

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

type verifyLine struct {
	Code        string  `json:"code,omitempty"`
	Description string  `json:"description,omitempty"`
	ServiceDate string  `json:"serviceDate,omitempty"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice,omitempty"`
}

// verifiedToolFixture seeds the nursing-note fixture (tenant, Tania, catalogue,
// 4 notes) and returns the verified create_invoice tool, the invoice service
// (to assert persistence) and an authed context.
func verifiedToolFixture(t *testing.T) (Tool, *service.InvoiceService, context.Context, int64) {
	t.Helper()
	conn, tenantID, participantID := noteToolsFixture(t)
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	notes := service.NewNoteService(conn, realtime.NewHub())
	seedReferenceNotes(t, notes, ctx, participantID)
	inv := service.NewInvoiceService(conn, realtime.NewHub())
	tool := NewCreateInvoiceToolVerified(inv, notes, nil)
	return tool, inv, ctx, participantID
}

func runVerifiedCreate(t *testing.T, tool Tool, ctx context.Context, participantID int64, items []verifyLine) (Result, error) {
	t.Helper()
	in := map[string]any{"participantId": participantID, "issueDate": "2026-06-14", "items": items}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return tool.Handler(ctx, raw)
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

func TestVerifyFullyCodedDraftSucceeds(t *testing.T) {
	tool, _, ctx, pid := verifiedToolFixture(t)
	res, err := runVerifiedCreate(t, tool, ctx, pid, fullCodedLines())
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
}

func TestVerifyMissingSelfCareLinesRejected(t *testing.T) {
	tool, inv, ctx, pid := verifiedToolFixture(t)
	// Only the transport lines — every self-care support is unbilled (run-2 bug).
	var items []verifyLine
	for i := range referenceWeek {
		items = append(items, verifyLine{Code: codeTransport, ServiceDate: referenceWeek[i].date, Quantity: referenceWeek[i].km})
	}
	_, err := runVerifiedCreate(t, tool, ctx, pid, items)
	if err == nil {
		t.Fatal("expected a completeness error for the omitted self-care lines")
	}
	if !strings.Contains(err.Error(), "support") {
		t.Fatalf("error %q should name the missing support hours", err.Error())
	}
	assertNoInvoice(t, inv, ctx)
}

func TestVerifyCustomLineInsteadOfCodeRejected(t *testing.T) {
	tool, inv, ctx, pid := verifiedToolFixture(t)
	// Transport coded, but self-care billed as a CUSTOM line (run-4 bug): right
	// quantities, wrong shape — no NDIS code.
	var items []verifyLine
	for i := range referenceWeek {
		d := referenceWeek[i]
		items = append(items,
			verifyLine{Code: codeTransport, ServiceDate: d.date, Quantity: d.km},
			verifyLine{Description: "self care", ServiceDate: d.date, Quantity: d.hr, UnitPrice: 65.0},
		)
	}
	_, err := runVerifiedCreate(t, tool, ctx, pid, items)
	if err == nil {
		t.Fatal("expected a completeness error: self-care billed as a custom line, not a code")
	}
	assertNoInvoice(t, inv, ctx)
}

// TestVerifyDroppedDayRejected covers P1-1: when the model bills only some days
// but declares the full note range (notesFrom/notesTo), the dropped day's
// supports are detected and the draft is rejected — even though the dropped day
// lies OUTSIDE the coded-line range.
func TestVerifyDroppedDayRejected(t *testing.T) {
	conn, tenantID, pid := noteToolsFixture(t)
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	notes := service.NewNoteService(conn, realtime.NewHub())
	seedReferenceNotes(t, notes, ctx, pid)
	inv := service.NewInvoiceService(conn, realtime.NewHub())
	tool := NewCreateInvoiceToolVerified(inv, notes, nil)

	// Bill only the first 3 days; drop 2026-06-12 entirely. Declare the full range.
	var items []verifyLine
	for i := 0; i < 3; i++ {
		d := referenceWeek[i]
		items = append(items,
			verifyLine{Code: codeTransport, ServiceDate: d.date, Quantity: d.km},
			verifyLine{Code: codeSelfCare, ServiceDate: d.date, Quantity: d.hr},
		)
	}
	in := map[string]any{
		"participantId": pid, "notesFrom": "2026-06-09", "notesTo": "2026-06-12", "items": items,
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := tool.Handler(ctx, raw); err == nil {
		t.Fatal("expected rejection: 2026-06-12 was dropped entirely")
	}
	assertNoInvoice(t, inv, ctx)
}

// TestVerifiedCreateBillsCoveredNotes covers P1-4: a successful notes→invoice
// create links every covered note to the new invoice (soft billing flag), so the
// flag the feature is built around is actually set in the agent flow.
func TestVerifiedCreateBillsCoveredNotes(t *testing.T) {
	conn, tenantID, pid := noteToolsFixture(t)
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	notes := service.NewNoteService(conn, realtime.NewHub())
	seedReferenceNotes(t, notes, ctx, pid)
	inv := service.NewInvoiceService(conn, realtime.NewHub())
	tool := NewCreateInvoiceToolVerified(inv, notes, nil)

	in := map[string]any{
		"participantId": pid, "notesFrom": "2026-06-09", "notesTo": "2026-06-12",
		"items": fullCodedLines(),
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res, err := tool.Handler(ctx, raw)
	if err != nil {
		t.Fatalf("create_invoice: %v", err)
	}
	created, ok := res.JSON.(*repository.Invoice)
	if !ok {
		t.Fatalf("result JSON is %T, want *repository.Invoice", res.JSON)
	}
	recs, err := notes.ListParticipant(ctx, pid, "2026-06-09", "2026-06-12")
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("no notes seeded")
	}
	for i := range recs { // bounded by len(recs)
		if recs[i].BilledID == nil || *recs[i].BilledID != created.ID {
			t.Fatalf("note %s not linked to invoice %d (billedId=%v)", recs[i].ServiceDate, created.ID, recs[i].BilledID)
		}
	}
}

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
