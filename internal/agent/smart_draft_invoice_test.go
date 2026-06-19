package agent

// Tests for the DraftInvoiceFromShifts Smart: gather → propose → apply with a
// bounded retry. They drive the deterministic apply over the real invoice/shift
// services (seeded from referenceWeek) while scripting the model with a *llm.Fake
// so the gather/propose/retry wiring is exercised without a live provider.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/shift"
)

// toolUse builds a forced-tool model response carrying a single create_invoice
// tool_use whose input is raw. It is a VALUE (Fake.SetResponses takes values).
func toolUse(name string, raw string) llm.Response {
	return llm.Response{
		StopReason: llm.StopToolUse,
		Content:    []llm.Block{{Type: llm.BlockToolUse, ToolName: name, Input: json.RawMessage(raw)}},
	}
}

// draftFixture builds a *Smarts wired over the real invoice + shift + catalogue
// services (seeded with referenceWeek shifts) and a scriptable *llm.Fake client.
func draftFixture(t *testing.T) (*Smarts, *llm.Fake, *invoice.Service, context.Context, int64) {
	t.Helper()
	conn, tenantID, participantID := shiftToolsFixture(t)
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	shifts := shift.NewService(conn, realtime.NewHub(), invoice.NewInvoices(conn))
	seedReferenceShifts(t, shifts, ctx, participantID)
	inv := invoice.NewService(conn, realtime.NewHub(), shift.NewShifts(conn))
	cat := catalog.NewService(conn)
	fake := llm.NewFake()
	s := &Smarts{client: fake, invoice: inv, shifts: shifts, catalog: cat}
	return s, fake, inv, ctx, participantID
}

// draftInput renders the create_invoice JSON for the given lines over the
// reference range; participantId/from/to are overwritten by the Smart from the
// URL so they need not be present here.
func draftInput(t *testing.T, items []verifyLine) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"issueDate": "2026-06-14",
		"items":     items,
	})
	if err != nil {
		t.Fatalf("marshal draft input: %v", err)
	}
	return string(raw)
}

// transportOnlyLines is a deliberately incomplete draft: it bills transport but
// omits every self-care (support hours) line, so applyDraftInvoice rejects it
// with a recoverable coverage error.
func transportOnlyLines() []verifyLine {
	out := make([]verifyLine, 0, len(referenceWeek))
	for i := range referenceWeek {
		out = append(out, verifyLine{Code: codeTransport, ServiceDate: referenceWeek[i].date, Quantity: referenceWeek[i].km})
	}
	return out
}

// TestDraftInvoiceFromShiftsRetrySucceeds scripts a failing first attempt
// (coverage gap) followed by a correct attempt, and asserts the Smart recovers
// and returns a persisted invoice.
func TestDraftInvoiceFromShiftsRetrySucceeds(t *testing.T) {
	s, fake, _, ctx, pid := draftFixture(t)
	fake.SetResponses(
		toolUse("create_invoice", draftInput(t, transportOnlyLines())),
		toolUse("create_invoice", draftInput(t, fullCodedLines())),
	)

	got, err := s.DraftInvoiceFromShifts(ctx, pid, "2026-06-09", "2026-06-12")
	if err != nil {
		t.Fatalf("DraftInvoiceFromShifts: %v", err)
	}
	if got == nil || got.ID <= 0 {
		t.Fatalf("want a persisted invoice with a positive id; got %+v", got)
	}
	if fake.Calls() != 2 {
		t.Fatalf("expected 2 model calls (one retry); got %d", fake.Calls())
	}

	// Every covered shift in range must now be linked to the new invoice
	// (billCoveredShifts → MarkDrafted swallows errors, so assert the effect).
	recs, err := s.shifts.ListParticipant(ctx, pid, "2026-06-09", "2026-06-12")
	if err != nil {
		t.Fatalf("list shifts: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("no shifts seeded in range")
	}
	for i := range recs {
		if recs[i].InvoiceID == nil || *recs[i].InvoiceID != got.ID {
			t.Fatalf("shift %s not linked to invoice %d (invoiceId=%v)", recs[i].ServiceDate, got.ID, recs[i].InvoiceID)
		}
	}
}

// TestDraftInvoiceFromShiftsRetriesEmptyItems proves an empty-items proposal is
// recoverable (errors.Is errRecoverableDraft) and re-proposed, not treated as
// fatal: attempt 1 has no items, attempt 2 is the correct full draft.
func TestDraftInvoiceFromShiftsRetriesEmptyItems(t *testing.T) {
	s, fake, _, ctx, pid := draftFixture(t)
	fake.SetResponses(
		toolUse("create_invoice", draftInput(t, nil)),
		toolUse("create_invoice", draftInput(t, fullCodedLines())),
	)

	got, err := s.DraftInvoiceFromShifts(ctx, pid, "2026-06-09", "2026-06-12")
	if err != nil {
		t.Fatalf("DraftInvoiceFromShifts: %v", err)
	}
	if got == nil || got.ID <= 0 {
		t.Fatalf("want a persisted invoice with a positive id; got %+v", got)
	}
	if fake.Calls() != 2 {
		t.Fatalf("expected 2 model calls (empty-items retried); got %d", fake.Calls())
	}
}

// TestDraftInvoiceFromShiftsRetryExhausted scripts two failing attempts and
// asserts the Smart gives up with a retry-exhaustion error and no orphan invoice.
func TestDraftInvoiceFromShiftsRetryExhausted(t *testing.T) {
	s, fake, inv, ctx, pid := draftFixture(t)
	fake.SetResponses(
		toolUse("create_invoice", draftInput(t, transportOnlyLines())),
		toolUse("create_invoice", draftInput(t, transportOnlyLines())),
		toolUse("create_invoice", draftInput(t, transportOnlyLines())),
	)

	_, err := s.DraftInvoiceFromShifts(ctx, pid, "2026-06-09", "2026-06-12")
	if err == nil {
		t.Fatal("expected a retry-exhaustion error after repeated failing attempts")
	}
	if !strings.Contains(err.Error(), "could not produce a valid invoice") {
		t.Fatalf("error %q should mention it could not produce a valid invoice", err.Error())
	}
	if fake.Calls() != maxDraftRetries+1 {
		t.Fatalf("expected %d model calls (loop bound); got %d", maxDraftRetries+1, fake.Calls())
	}
	assertNoInvoice(t, inv, ctx)
}

// TestDraftInvoiceSearchesCatalogueThenBillsWithNarrative drives the redesigned
// flow: the model calls the read-only search_catalogue tool to ground its codes
// (turn 1), then emits create_invoice with per-line descriptions taken from the
// shift notes (turn 2). Asserts the search was actually run, the invoice was
// created, and the line descriptions are the model's narrative — NOT the generic
// catalogue item name the validator would otherwise backfill.
func TestDraftInvoiceSearchesCatalogueThenBillsWithNarrative(t *testing.T) {
	s, fake, inv, ctx, pid := draftFixture(t)

	items := make([]map[string]any, 0, len(referenceWeek)*2)
	for i := range referenceWeek {
		d := referenceWeek[i]
		items = append(items,
			map[string]any{"code": codeSelfCare, "serviceDate": d.date, "quantity": d.hr,
				"description": fmt.Sprintf("Self-care support for Tania on %s", d.date)},
			map[string]any{"code": codeTransport, "serviceDate": d.date, "quantity": d.km,
				"description": fmt.Sprintf("Community access transport, %.0f km", d.km)},
		)
	}
	createRaw, err := json.Marshal(map[string]any{"issueDate": "2026-06-14", "items": items})
	if err != nil {
		t.Fatalf("marshal create input: %v", err)
	}
	fake.SetResponses(
		toolUse("search_catalogue", `{"query":"self care","serviceDate":"2026-06-09"}`),
		toolUse("create_invoice", string(createRaw)),
	)

	got, err := s.DraftInvoiceFromShifts(ctx, pid, "2026-06-09", "2026-06-12")
	if err != nil {
		t.Fatalf("DraftInvoiceFromShifts: %v", err)
	}
	if got == nil || got.ID == 0 {
		t.Fatal("no invoice created")
	}
	if fake.Calls() != 2 {
		t.Fatalf("model calls = %d, want 2 (search then create)", fake.Calls())
	}

	full, err := inv.Get(ctx, got.ID)
	if err != nil {
		t.Fatalf("get invoice: %v", err)
	}
	if len(full.LineItems) != len(referenceWeek)*2 {
		t.Fatalf("line items = %d, want %d", len(full.LineItems), len(referenceWeek)*2)
	}
	for _, li := range full.LineItems {
		if li.Description == "Assistance with self care - weekday daytime" || li.Description == "Activity Based Transport" {
			t.Fatalf("line fell back to the catalogue name instead of the narrative: %q", li.Description)
		}
		if !strings.Contains(li.Description, "Tania") && !strings.Contains(li.Description, "transport") {
			t.Fatalf("line description is not the service narrative: %q", li.Description)
		}
	}
}
