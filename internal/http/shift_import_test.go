package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent"
	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

// newImportHandler builds an enabled AgentHandler with shift import wired over a
// migrated temp DB, plus a request context carrying a tenant+user so guard
// passes. The fake llm is never reached by the validation tests (they return
// before any model call).
func newImportHandler(t *testing.T) (*AgentHandler, context.Context) {
	t.Helper()
	conn := openMigratedDB(t, "shift_import.db")
	_, tenantID, userID := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	store := agent.NewStore(conn)
	cp := agent.NewCheckpoint(store, conn)
	reg := agent.NewRegistry()
	events := agent.NewEvents()
	cfg := agent.Config{APIKey: "test"}.WithDefaults()
	budget := agent.NewBudget(store, cfg, clockNow{})
	ag := agent.NewAgent(cfg, llm.NewFake(), store, reg, cp, events).WithBudget(budget)

	shiftSvc := service.NewShiftService(conn, hub)
	h := NewAgentHandler(ag, budget, true).WithShiftImport(shiftSvc, llm.NewFake(), cfg)

	ctx := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)
	return h, ctx
}

func importReq(t *testing.T, ctx context.Context, body string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/shifts/import", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	return r.WithContext(ctx)
}

func TestImportShiftsMissingParticipant400(t *testing.T) {
	h, ctx := newImportHandler(t)
	w := httptest.NewRecorder()
	h.ImportShifts(w, importReq(t, ctx, `{"text":"worked Monday 5h"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing participant: want 400 got %d (%s)", w.Code, w.Body.String())
	}
}

func TestImportShiftsEmptyText400(t *testing.T) {
	h, ctx := newImportHandler(t)
	w := httptest.NewRecorder()
	h.ImportShifts(w, importReq(t, ctx, `{"participantId":1,"text":"   "}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty text: want 400 got %d (%s)", w.Code, w.Body.String())
	}
}

func TestImportShiftsBadBody400(t *testing.T) {
	h, ctx := newImportHandler(t)
	w := httptest.NewRecorder()
	h.ImportShifts(w, importReq(t, ctx, `{not json}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad body: want 400 got %d (%s)", w.Code, w.Body.String())
	}
}

func TestImportShiftsDisabled503(t *testing.T) {
	// A guard-only (disabled) handler 503s before any validation.
	h := NewAgentHandler(nil, nil, false)
	w := httptest.NewRecorder()
	h.ImportShifts(w, importReq(t, context.Background(), `{"participantId":1,"text":"x"}`))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled: want 503 got %d", w.Code)
	}
}
