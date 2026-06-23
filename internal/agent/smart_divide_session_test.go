package agent

// Tests for the DivideSession Smart: gather → propose → apply with a bounded
// retry. They drive the deterministic apply over the real session service (seeded
// with a note-only reference session) while scripting the model with a *llm.Fake so
// the gather/propose/retry wiring is exercised without a live provider.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/pricelist"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/session"
)

// toolUse builds a forced-tool model response carrying a single tool_use whose
// input is raw. It is a VALUE (Fake.SetResponses takes values).
func toolUse(name string, raw string) llm.Response {
	return llm.Response{
		StopReason: llm.StopToolUse,
		Content:    []llm.Block{{Type: llm.BlockToolUse, ToolName: name, Input: json.RawMessage(raw)}},
	}
}

// divideFixture builds a *Smarts wired over the real session + catalogue services
// (with one seeded note-only reference session) and a scriptable *llm.Fake client,
// returning the Smarts, the fake, the session service, an authed context and the
// seeded session id.
func divideFixture(t *testing.T) (*Smarts, *llm.Fake, *session.Service, context.Context, int64) {
	t.Helper()
	conn, tenantID, clientID := sessionToolsFixture(t)
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	sessions := session.NewService(conn, conn, realtime.NewHub(), invoice.NewInvoices(conn))
	sh := seedReferenceSession(t, sessions, ctx, clientID, referenceWeek[0].date)
	cat := pricelist.NewService(conn)
	fake := llm.NewFake()
	s := &Smarts{client: fake, sessions: sessions, catalog: cat}
	return s, fake, sessions, ctx, sh.ID
}

// divideLine is one divide_session item; serviceDate is intentionally omitted (the
// session service stamps the session's date).
type divideLine struct {
	Code        string  `json:"code,omitempty"`
	Description string  `json:"description,omitempty"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice,omitempty"`
}

// divideInput renders the divide_session JSON for the given lines.
func divideInput(t *testing.T, items []divideLine) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatalf("marshal divide input: %v", err)
	}
	return string(raw)
}

// codedLines is a valid two-line divide: a self-care support line + a transport
// line, both catalogue-coded (prices resolved by the session service).
func codedLines() []divideLine {
	return []divideLine{
		{Code: codeSelfCare, Quantity: referenceWeek[0].hr, Description: "Self-care support for Tania"},
		{Code: codeTransport, Quantity: referenceWeek[0].km, Description: "Community access transport"},
	}
}

// TestDivideSessionPersistsPricedItems scripts a single divide_session call carrying
// a self-care H line + a transport KM line and asserts both are persisted on the
// session, priced from the catalogue, with invoice_id NULL.
func TestDivideSessionPersistsPricedItems(t *testing.T) {
	s, fake, sessions, ctx, sessionID := divideFixture(t)
	fake.SetResponses(toolUse("divide_session", divideInput(t, codedLines())))

	if err := s.DivideSession(ctx, sessionID); err != nil {
		t.Fatalf("DivideSession: %v", err)
	}
	if fake.Calls() != 1 {
		t.Fatalf("model calls = %d, want 1", fake.Calls())
	}

	items, err := sessions.ListItems(ctx, sessionID)
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
		if it.SessionID == nil || *it.SessionID != sessionID {
			t.Fatalf("item %d sessionId = %v, want %d", i, it.SessionID, sessionID)
		}
		if it.Code == "" {
			t.Fatalf("item %d should be catalogue-coded", i)
		}
		if it.UnitPrice <= 0 {
			t.Fatalf("item %d (code %q) should be priced from the catalogue, got %.2f", i, it.Code, it.UnitPrice)
		}
	}
}

// TestDivideSessionSearchesCatalogueThenDivides drives the redesigned flow: the
// model calls the read-only search_catalogue tool to ground its codes (turn 1),
// then emits divide_session (turn 2). Asserts the search ran and the items
// persisted with their narrative descriptions (not the catalogue name).
func TestDivideSessionSearchesCatalogueThenDivides(t *testing.T) {
	s, fake, sessions, ctx, sessionID := divideFixture(t)
	fake.SetResponses(
		toolUse("search_catalogue", `{"query":"self care","serviceDate":"2026-06-09"}`),
		toolUse("divide_session", divideInput(t, codedLines())),
	)

	if err := s.DivideSession(ctx, sessionID); err != nil {
		t.Fatalf("DivideSession: %v", err)
	}
	if fake.Calls() != 2 {
		t.Fatalf("model calls = %d, want 2 (search then divide)", fake.Calls())
	}

	items, err := sessions.ListItems(ctx, sessionID)
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

// TestDivideSessionReplacesUnbilledItems asserts a re-divide is idempotent: it
// clears the prior unbilled items before adding the new ones (no accumulation).
func TestDivideSessionReplacesUnbilledItems(t *testing.T) {
	s, fake, sessions, ctx, sessionID := divideFixture(t)
	fake.SetResponses(
		toolUse("divide_session", divideInput(t, codedLines())),
		toolUse("divide_session", divideInput(t, codedLines()[:1])), // re-divide → 1 line
	)

	if err := s.DivideSession(ctx, sessionID); err != nil {
		t.Fatalf("first DivideSession: %v", err)
	}
	if err := s.DivideSession(ctx, sessionID); err != nil {
		t.Fatalf("second DivideSession: %v", err)
	}
	items, err := sessions.ListItems(ctx, sessionID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("after re-divide items = %d, want 1 (replaced, not appended)", len(items))
	}
}

// TestDivideSessionRetriesEmptyItems proves an empty-items proposal is recoverable
// (errors.Is errRecoverableDivide) and re-proposed: attempt 1 has no items,
// attempt 2 is the correct divide.
func TestDivideSessionRetriesEmptyItems(t *testing.T) {
	s, fake, sessions, ctx, sessionID := divideFixture(t)
	fake.SetResponses(
		toolUse("divide_session", divideInput(t, nil)),
		toolUse("divide_session", divideInput(t, codedLines())),
	)

	if err := s.DivideSession(ctx, sessionID); err != nil {
		t.Fatalf("DivideSession: %v", err)
	}
	if fake.Calls() != 2 {
		t.Fatalf("model calls = %d, want 2 (empty-items retried)", fake.Calls())
	}
	items, err := sessions.ListItems(ctx, sessionID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
}

// TestDivideSessionRetryExhausted scripts repeated empty-items proposals and
// asserts the Smart gives up with a retry-exhaustion error and leaves no items.
func TestDivideSessionRetryExhausted(t *testing.T) {
	s, fake, sessions, ctx, sessionID := divideFixture(t)
	empty := divideInput(t, nil)
	fake.SetResponses(
		toolUse("divide_session", empty),
		toolUse("divide_session", empty),
		toolUse("divide_session", empty),
	)

	err := s.DivideSession(ctx, sessionID)
	if err == nil {
		t.Fatal("expected a retry-exhaustion error after repeated empty proposals")
	}
	if !strings.Contains(err.Error(), "could not produce valid items") {
		t.Fatalf("error %q should mention it could not produce valid items", err.Error())
	}
	if fake.Calls() != maxDivideRetries+1 {
		t.Fatalf("model calls = %d, want %d (loop bound)", fake.Calls(), maxDivideRetries+1)
	}
	items, err := sessions.ListItems(ctx, sessionID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("a failed divide must leave no items; found %d", len(items))
	}
}
