package agent

// Tests for the DivideShift Smart: gather → propose → apply with a bounded
// retry. They drive the deterministic apply over the real shift service (seeded
// with a note-only reference shift) while scripting the model with a *llm.Fake so
// the gather/propose/retry wiring is exercised without a live provider.

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

// toolUse builds a forced-tool model response carrying a single tool_use whose
// input is raw. It is a VALUE (Fake.SetResponses takes values).
func toolUse(name string, raw string) llm.Response {
	return llm.Response{
		StopReason: llm.StopToolUse,
		Content:    []llm.Block{{Type: llm.BlockToolUse, ToolName: name, Input: json.RawMessage(raw)}},
	}
}

// divideFixture builds a *Smarts wired over the real shift + catalogue services
// (with one seeded note-only reference shift) and a scriptable *llm.Fake client,
// returning the Smarts, the fake, the shift service, an authed context and the
// seeded shift id.
func divideFixture(t *testing.T) (*Smarts, *llm.Fake, *shift.Service, context.Context, int64) {
	t.Helper()
	conn, tenantID, clientID := shiftToolsFixture(t)
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	shifts := shift.NewService(conn, conn, realtime.NewHub(), invoice.NewInvoices(conn))
	sh := seedReferenceShift(t, shifts, ctx, clientID, referenceWeek[0].date)
	cat := catalog.NewService(conn)
	fake := llm.NewFake()
	s := &Smarts{client: fake, shifts: shifts, catalog: cat}
	return s, fake, shifts, ctx, sh.ID
}

// divideLine is one divide_shift item; serviceDate is intentionally omitted (the
// shift service stamps the shift's date).
type divideLine struct {
	Code        string  `json:"code,omitempty"`
	Description string  `json:"description,omitempty"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice,omitempty"`
}

// divideInput renders the divide_shift JSON for the given lines.
func divideInput(t *testing.T, items []divideLine) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatalf("marshal divide input: %v", err)
	}
	return string(raw)
}

// codedLines is a valid two-line divide: a self-care support line + a transport
// line, both catalogue-coded (prices resolved by the shift service).
func codedLines() []divideLine {
	return []divideLine{
		{Code: codeSelfCare, Quantity: referenceWeek[0].hr, Description: "Self-care support for Tania"},
		{Code: codeTransport, Quantity: referenceWeek[0].km, Description: "Community access transport"},
	}
}

// TestDivideShiftPersistsPricedItems scripts a single divide_shift call carrying
// a self-care H line + a transport KM line and asserts both are persisted on the
// shift, priced from the catalogue, with invoice_id NULL.
func TestDivideShiftPersistsPricedItems(t *testing.T) {
	s, fake, shifts, ctx, shiftID := divideFixture(t)
	fake.SetResponses(toolUse("divide_shift", divideInput(t, codedLines())))

	if err := s.DivideShift(ctx, shiftID); err != nil {
		t.Fatalf("DivideShift: %v", err)
	}
	if fake.Calls() != 1 {
		t.Fatalf("model calls = %d, want 1", fake.Calls())
	}

	items, err := shifts.ListItems(ctx, shiftID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	for i := range items {
		it := items[i]
		if it.InvoiceID != nil {
			t.Fatalf("item %d should be unbilled (invoiceId nil), got %v", i, it.InvoiceID)
		}
		if it.ShiftID == nil || *it.ShiftID != shiftID {
			t.Fatalf("item %d shiftId = %v, want %d", i, it.ShiftID, shiftID)
		}
		if it.Code == "" {
			t.Fatalf("item %d should be catalogue-coded", i)
		}
		if it.UnitPrice <= 0 {
			t.Fatalf("item %d (code %q) should be priced from the catalogue, got %.2f", i, it.Code, it.UnitPrice)
		}
	}
}

// TestDivideShiftSearchesCatalogueThenDivides drives the redesigned flow: the
// model calls the read-only search_catalogue tool to ground its codes (turn 1),
// then emits divide_shift (turn 2). Asserts the search ran and the items
// persisted with their narrative descriptions (not the catalogue name).
func TestDivideShiftSearchesCatalogueThenDivides(t *testing.T) {
	s, fake, shifts, ctx, shiftID := divideFixture(t)
	fake.SetResponses(
		toolUse("search_catalogue", `{"query":"self care","serviceDate":"2026-06-09"}`),
		toolUse("divide_shift", divideInput(t, codedLines())),
	)

	if err := s.DivideShift(ctx, shiftID); err != nil {
		t.Fatalf("DivideShift: %v", err)
	}
	if fake.Calls() != 2 {
		t.Fatalf("model calls = %d, want 2 (search then divide)", fake.Calls())
	}

	items, err := shifts.ListItems(ctx, shiftID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	for i := range items {
		d := items[i].Description
		if d == "Assistance with self care - weekday daytime" || d == "Activity Based Transport" {
			t.Fatalf("line %d fell back to the catalogue name instead of the narrative: %q", i, d)
		}
	}
}

// TestDivideShiftReplacesUnbilledItems asserts a re-divide is idempotent: it
// clears the prior unbilled items before adding the new ones (no accumulation).
func TestDivideShiftReplacesUnbilledItems(t *testing.T) {
	s, fake, shifts, ctx, shiftID := divideFixture(t)
	fake.SetResponses(
		toolUse("divide_shift", divideInput(t, codedLines())),
		toolUse("divide_shift", divideInput(t, codedLines()[:1])), // re-divide → 1 line
	)

	if err := s.DivideShift(ctx, shiftID); err != nil {
		t.Fatalf("first DivideShift: %v", err)
	}
	if err := s.DivideShift(ctx, shiftID); err != nil {
		t.Fatalf("second DivideShift: %v", err)
	}
	items, err := shifts.ListItems(ctx, shiftID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("after re-divide items = %d, want 1 (replaced, not appended)", len(items))
	}
}

// TestDivideShiftRetriesEmptyItems proves an empty-items proposal is recoverable
// (errors.Is errRecoverableDivide) and re-proposed: attempt 1 has no items,
// attempt 2 is the correct divide.
func TestDivideShiftRetriesEmptyItems(t *testing.T) {
	s, fake, shifts, ctx, shiftID := divideFixture(t)
	fake.SetResponses(
		toolUse("divide_shift", divideInput(t, nil)),
		toolUse("divide_shift", divideInput(t, codedLines())),
	)

	if err := s.DivideShift(ctx, shiftID); err != nil {
		t.Fatalf("DivideShift: %v", err)
	}
	if fake.Calls() != 2 {
		t.Fatalf("model calls = %d, want 2 (empty-items retried)", fake.Calls())
	}
	items, err := shifts.ListItems(ctx, shiftID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
}

// TestDivideShiftRetryExhausted scripts repeated empty-items proposals and
// asserts the Smart gives up with a retry-exhaustion error and leaves no items.
func TestDivideShiftRetryExhausted(t *testing.T) {
	s, fake, shifts, ctx, shiftID := divideFixture(t)
	empty := divideInput(t, nil)
	fake.SetResponses(
		toolUse("divide_shift", empty),
		toolUse("divide_shift", empty),
		toolUse("divide_shift", empty),
	)

	err := s.DivideShift(ctx, shiftID)
	if err == nil {
		t.Fatal("expected a retry-exhaustion error after repeated empty proposals")
	}
	if !strings.Contains(err.Error(), "could not produce valid items") {
		t.Fatalf("error %q should mention it could not produce valid items", err.Error())
	}
	if fake.Calls() != maxDivideRetries+1 {
		t.Fatalf("model calls = %d, want %d (loop bound)", fake.Calls(), maxDivideRetries+1)
	}
	items, err := shifts.ListItems(ctx, shiftID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("a failed divide must leave no items; found %d", len(items))
	}
}
