package agent

// Tests for the DraftInvoiceFromShifts Smart: gather → propose → apply with a
// bounded retry. They drive the deterministic apply over the real invoice/shift
// services (seeded from referenceWeek) while scripting the model with a *llm.Fake
// so the gather/propose/retry wiring is exercised without a live provider.

import (
	"context"
	"encoding/json"
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
	assertNoInvoice(t, inv, ctx)
}
