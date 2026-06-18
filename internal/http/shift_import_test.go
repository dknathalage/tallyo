package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent"
	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
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

// emitShiftsResp builds a scripted forced emit_shifts tool_use response carrying
// the given drafts, mirroring the model's structured-extraction output.
func emitShiftsResp(t *testing.T, drafts []agent.ShiftDraft) llm.Response {
	t.Helper()
	in, err := json.Marshal(map[string]any{"shifts": drafts})
	if err != nil {
		t.Fatalf("marshal emit_shifts input: %v", err)
	}
	return llm.Response{
		StopReason: llm.StopToolUse,
		Content: []llm.Block{{
			Type:      llm.BlockToolUse,
			ToolUseID: "toolu_import_1",
			ToolName:  "emit_shifts",
			Input:     json.RawMessage(in),
		}},
	}
}

// TestImportShiftsIdempotent asserts that importing the same timesheet twice does
// not create duplicate shifts: the second run extracts the same drafts but every
// one already exists, so zero new shifts are created and the participant ends with
// exactly one shift per extracted day.
func TestImportShiftsIdempotent(t *testing.T) {
	conn := openMigratedDB(t, "shift_import_idem.db")
	_, tenantID, userID := seedTenantOwner(t, conn)

	tctx := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)
	p, err := repository.NewParticipants(conn).Create(tctx, tenantID, repository.ParticipantInput{Name: "Tania"})
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}

	drafts := []agent.ShiftDraft{
		{ServiceDate: "2026-06-09", StartTime: "09:00", EndTime: "16:00", Hours: 7, Km: 36, Note: "self care"},
		{ServiceDate: "2026-06-10", StartTime: "11:30", EndTime: "17:00", Hours: 5.5, Km: 12, Note: "self care"},
	}
	// Two identical extractions: one per ImportShifts call.
	fake := llm.NewFake(emitShiftsResp(t, drafts), emitShiftsResp(t, drafts))

	store := agent.NewStore(conn)
	cp := agent.NewCheckpoint(store, conn)
	reg := agent.NewRegistry()
	events := agent.NewEvents()
	cfg := agent.Config{APIKey: "test"}.WithDefaults()
	budget := agent.NewBudget(store, cfg, clockNow{})
	ag := agent.NewAgent(cfg, llm.NewFake(), store, reg, cp, events).WithBudget(budget)

	shiftSvc := service.NewShiftService(conn, realtime.NewHub())
	h := NewAgentHandler(ag, budget, true).WithShiftImport(shiftSvc, fake, cfg)

	body := `{"participantId":` + itoa(p.ID) + `,"text":"timesheet"}`

	w1 := httptest.NewRecorder()
	h.ImportShifts(w1, importReq(t, tctx, body))
	if w1.Code != http.StatusCreated {
		t.Fatalf("first import: want 201 got %d (%s)", w1.Code, w1.Body.String())
	}
	var first []*repository.Shift
	if err := json.Unmarshal(w1.Body.Bytes(), &first); err != nil {
		t.Fatalf("first import: decode: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first import created %d shifts, want 2", len(first))
	}

	w2 := httptest.NewRecorder()
	h.ImportShifts(w2, importReq(t, tctx, body))
	if w2.Code != http.StatusCreated {
		t.Fatalf("second import: want 201 got %d (%s)", w2.Code, w2.Body.String())
	}
	var second []*repository.Shift
	if err := json.Unmarshal(w2.Body.Bytes(), &second); err != nil {
		t.Fatalf("second import: decode: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second import created %d shifts, want 0 (all already exist)", len(second))
	}

	all, err := shiftSvc.ListParticipant(tctx, p.ID, "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("participant has %d shifts after two imports, want 2 (no duplicates)", len(all))
	}
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
