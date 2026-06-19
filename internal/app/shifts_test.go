package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/go-chi/chi/v5"
)

// shiftFixture bundles a migrated DB, the shift handler under test, the seeded
// tenant context, and the seeded participant id. The handler is exercised
// directly (no router) by injecting the tenant context plus chi {id} param.
type shiftFixture struct {
	h             *shift.Handler
	ctx           context.Context
	tenantID      int64
	participantID int64
}

// newShiftFixture migrates a temp DB, seeds a tenant+participant, and returns a
// handler wired to the real ShiftService.
func newShiftFixture(t *testing.T) *shiftFixture {
	t.Helper()
	conn := openMigratedDB(t, "shift.db")
	_, tenantID, _ := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	ctx := reqctx.WithTenant(context.Background(), tenantID)

	partSvc := participant.NewService(conn, hub)
	part, err := partSvc.Create(ctx, participant.ParticipantInput{Name: "Stark"})
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}
	if part == nil || part.ID == 0 {
		t.Fatalf("seed participant: want id>0 got %+v", part)
	}

	return &shiftFixture{
		h:             shift.NewHandler(shift.NewService(conn, hub, invoice.NewInvoices(conn)), nil),
		ctx:           ctx,
		tenantID:      tenantID,
		participantID: part.ID,
	}
}

// req builds a request carrying the tenant context and the {id} chi URL param.
func (f *shiftFixture) req(t *testing.T, method, url, idParam, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, url, nil)
	} else {
		r = httptest.NewRequest(method, url, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r = r.WithContext(f.ctx)
	if idParam != "" {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", idParam)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}
	return r
}

// createShift posts a shift via the handler and returns the decoded shift.
func (f *shiftFixture) createShift(t *testing.T, body string) shift.Shift {
	t.Helper()
	w := httptest.NewRecorder()
	f.h.Create(w, f.req(t, http.MethodPost, "/api/shifts", "", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create shift: want 201 got %d (%s)", w.Code, w.Body.String())
	}
	var out shift.Shift
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode shift: %v", err)
	}
	return out
}

func TestShiftCreateRoundTripsFields(t *testing.T) {
	f := newShiftFixture(t)
	body, err := json.Marshal(map[string]any{
		"participantId": f.participantID,
		"serviceDate":   "2026-01-05",
		"note":          "Took client shopping.",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := f.createShift(t, string(body))
	if s.ID == 0 {
		t.Fatalf("create: want id>0 got %d", s.ID)
	}
	if s.ParticipantID != f.participantID {
		t.Fatalf("participantId: want %d got %d", f.participantID, s.ParticipantID)
	}
	if s.ServiceDate != "2026-01-05" {
		t.Fatalf("serviceDate: want 2026-01-05 got %q", s.ServiceDate)
	}
	if s.Status != "recorded" {
		t.Fatalf("status: want recorded got %q", s.Status)
	}
}

func TestShiftCreateMissingServiceDate400(t *testing.T) {
	f := newShiftFixture(t)
	body, _ := json.Marshal(map[string]any{"participantId": f.participantID})
	w := httptest.NewRecorder()
	f.h.Create(w, f.req(t, http.MethodPost, "/api/shifts", "", string(body)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing service date: want 400 got %d", w.Code)
	}
}

func TestShiftListForParticipantEmptyReturnsArray(t *testing.T) {
	f := newShiftFixture(t)
	w := httptest.NewRecorder()
	f.h.ListForParticipant(w, f.req(t, http.MethodGet, "/api/participants/1/shifts", itoa(f.participantID), ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", w.Code)
	}
	if got := w.Body.String(); got != "[]\n" {
		t.Fatalf("empty list: want %q got %q", "[]\n", got)
	}
}

func TestShiftListForParticipantReturnsCreated(t *testing.T) {
	f := newShiftFixture(t)
	body, _ := json.Marshal(map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-05", "note": "Entry.",
	})
	created := f.createShift(t, string(body))

	w := httptest.NewRecorder()
	f.h.ListForParticipant(w, f.req(t, http.MethodGet, "/api/participants/1/shifts", itoa(f.participantID), ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", w.Code)
	}
	var out []shift.Shift
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 || out[0].ID != created.ID {
		t.Fatalf("list: want [%d] got %+v", created.ID, out)
	}
}

func TestShiftListForParticipantRangeFilter(t *testing.T) {
	f := newShiftFixture(t)
	in := f.createShift(t, mustJSON(t, map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-10", "note": "in-range",
	}))
	_ = f.createShift(t, mustJSON(t, map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-02-20", "note": "out-of-range",
	}))

	w := httptest.NewRecorder()
	f.h.ListForParticipant(w, f.req(t, http.MethodGet,
		"/api/participants/1/shifts?from=2026-01-01&to=2026-01-31", itoa(f.participantID), ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list range: want 200 got %d", w.Code)
	}
	var out []shift.Shift
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 || out[0].ID != in.ID {
		t.Fatalf("range filter: want [%d] got %+v", in.ID, out)
	}
}

func TestShiftListForParticipantStatusFilter(t *testing.T) {
	f := newShiftFixture(t)
	rec := f.createShift(t, mustJSON(t, map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-10", "status": "recorded",
	}))
	sched := f.createShift(t, mustJSON(t, map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-11", "status": "scheduled",
	}))
	_ = sched

	w := httptest.NewRecorder()
	f.h.ListForParticipant(w, f.req(t, http.MethodGet,
		"/api/participants/1/shifts?status=recorded", itoa(f.participantID), ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list status: want 200 got %d", w.Code)
	}
	var out []shift.Shift
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 || out[0].ID != rec.ID {
		t.Fatalf("status filter: want [%d] got %+v", rec.ID, out)
	}
}

func TestShiftListAllTenant(t *testing.T) {
	f := newShiftFixture(t)
	_ = f.createShift(t, mustJSON(t, map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-10",
	}))
	_ = f.createShift(t, mustJSON(t, map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-02-20",
	}))

	w := httptest.NewRecorder()
	f.h.List(w, f.req(t, http.MethodGet, "/api/shifts", "", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list all: want 200 got %d", w.Code)
	}
	var out []shift.Shift
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("list all: want 2 got %d", len(out))
	}
}

func TestShiftGetUnknown404(t *testing.T) {
	f := newShiftFixture(t)
	w := httptest.NewRecorder()
	f.h.Get(w, f.req(t, http.MethodGet, "/api/shifts/99999", "99999", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("get unknown: want 404 got %d", w.Code)
	}
}

func TestShiftUpdateUnknown404(t *testing.T) {
	f := newShiftFixture(t)
	body, _ := json.Marshal(map[string]any{"serviceDate": "2026-01-06", "note": "Nope."})
	w := httptest.NewRecorder()
	f.h.Update(w, f.req(t, http.MethodPut, "/api/shifts/99999", "99999", string(body)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("update unknown: want 404 got %d", w.Code)
	}
}

func TestShiftStatusTransition(t *testing.T) {
	f := newShiftFixture(t)
	created := f.createShift(t, mustJSON(t, map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-05", "status": "scheduled",
	}))

	w := httptest.NewRecorder()
	f.h.UpdateStatus(w, f.req(t, http.MethodPost, "/api/shifts/1/status",
		itoa(created.ID), mustJSON(t, map[string]any{"status": "recorded"})))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status transition: want 204 got %d (%s)", w.Code, w.Body.String())
	}

	gw := httptest.NewRecorder()
	f.h.Get(gw, f.req(t, http.MethodGet, "/api/shifts/1", itoa(created.ID), ""))
	if gw.Code != http.StatusOK {
		t.Fatalf("get after status: want 200 got %d", gw.Code)
	}
	var got shift.Shift
	if err := json.NewDecoder(gw.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "recorded" {
		t.Fatalf("status: want recorded got %q", got.Status)
	}
}

func TestShiftStatusEmpty400(t *testing.T) {
	f := newShiftFixture(t)
	created := f.createShift(t, mustJSON(t, map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-05",
	}))
	w := httptest.NewRecorder()
	f.h.UpdateStatus(w, f.req(t, http.MethodPost, "/api/shifts/1/status",
		itoa(created.ID), mustJSON(t, map[string]any{"status": ""})))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty status: want 400 got %d", w.Code)
	}
}

func TestShiftDelete204(t *testing.T) {
	f := newShiftFixture(t)
	created := f.createShift(t, mustJSON(t, map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-05",
	}))

	w := httptest.NewRecorder()
	f.h.Delete(w, f.req(t, http.MethodDelete, "/api/shifts/1", itoa(created.ID), ""))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", w.Code)
	}
}

func TestShiftSuggestionsAndToRecordEmpty(t *testing.T) {
	f := newShiftFixture(t)

	sw := httptest.NewRecorder()
	f.h.Suggestions(sw, f.req(t, http.MethodGet, "/api/shifts/suggestions", "", ""))
	if sw.Code != http.StatusOK {
		t.Fatalf("suggestions: want 200 got %d", sw.Code)
	}
	if got := sw.Body.String(); got != "[]\n" {
		t.Fatalf("suggestions empty: want %q got %q", "[]\n", got)
	}

	tw := httptest.NewRecorder()
	f.h.ToRecord(tw, f.req(t, http.MethodGet, "/api/shifts/to-record", "", ""))
	if tw.Code != http.StatusOK {
		t.Fatalf("to-record: want 200 got %d", tw.Code)
	}
	if got := tw.Body.String(); got != "[]\n" {
		t.Fatalf("to-record empty: want %q got %q", "[]\n", got)
	}
}

// mustJSON marshals v to a JSON string or fails the test.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
