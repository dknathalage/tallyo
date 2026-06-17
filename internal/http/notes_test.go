package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// noteFixture bundles a migrated DB, the note handler under test, the seeded
// tenant context, and the seeded participant id. The handler is exercised
// directly (no router) by injecting the tenant context plus chi {id} param.
type noteFixture struct {
	h             *NoteHandler
	ctx           context.Context
	tenantID      int64
	participantID int64
	invoiceID     int64
}

// newNoteFixture migrates a temp DB, seeds a tenant+participant (and one invoice
// for the billing test), and returns a handler wired to the real NoteService.
func newNoteFixture(t *testing.T) *noteFixture {
	t.Helper()
	conn := openMigratedDB(t, "note.db")
	_, tenantID, _ := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	ctx := reqctx.WithTenant(context.Background(), tenantID)

	partSvc := service.NewParticipantService(conn, hub)
	part, err := partSvc.Create(ctx, repository.ParticipantInput{Name: "Stark"})
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}
	if part == nil || part.ID == 0 {
		t.Fatalf("seed participant: want id>0 got %+v", part)
	}

	invSvc := service.NewInvoiceService(conn, hub)
	inv, err := invSvc.Create(ctx, repository.InvoiceInput{
		ParticipantID: part.ID, Status: "draft", IssueDate: "2026-01-01", DueDate: "2026-01-31",
	}, []repository.LineItemInput{{Description: "Support", Quantity: 1, UnitPrice: 100}})
	if err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	if inv == nil || inv.ID == 0 {
		t.Fatalf("seed invoice: want id>0 got %+v", inv)
	}

	return &noteFixture{
		h:             NewNoteHandler(service.NewNoteService(conn, hub)),
		ctx:           ctx,
		tenantID:      tenantID,
		participantID: part.ID,
		invoiceID:     inv.ID,
	}
}

// req builds a request carrying the tenant context and the {id} chi URL param.
func (f *noteFixture) req(t *testing.T, method, url, idParam, body string) *http.Request {
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

// createNote posts a note via the handler and returns the decoded note.
func (f *noteFixture) createNote(t *testing.T, body string) repository.Note {
	t.Helper()
	w := httptest.NewRecorder()
	f.h.Create(w, f.req(t, http.MethodPost, "/api/notes", "", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create note: want 201 got %d (%s)", w.Code, w.Body.String())
	}
	var out repository.Note
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode note: %v", err)
	}
	return out
}

func TestNoteCreateRoundTripsFields(t *testing.T) {
	f := newNoteFixture(t)
	body, err := json.Marshal(map[string]any{
		"participantId": f.participantID,
		"serviceDate":   "2026-01-05",
		"body":          "Took client shopping.",
		"transportKm":   12.5,
		"supportHours":  2.0,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	n := f.createNote(t, string(body))
	if n.ID == 0 {
		t.Fatalf("create: want id>0 got %d", n.ID)
	}
	if n.ParticipantID != f.participantID {
		t.Fatalf("participantId: want %d got %d", f.participantID, n.ParticipantID)
	}
	if n.ServiceDate != "2026-01-05" {
		t.Fatalf("serviceDate: want 2026-01-05 got %q", n.ServiceDate)
	}
	if n.Body != "Took client shopping." {
		t.Fatalf("body: want %q got %q", "Took client shopping.", n.Body)
	}
	if n.TransportKm == nil || *n.TransportKm != 12.5 {
		t.Fatalf("transportKm: want 12.5 got %v", n.TransportKm)
	}
	if n.SupportHours == nil || *n.SupportHours != 2.0 {
		t.Fatalf("supportHours: want 2 got %v", n.SupportHours)
	}
}

func TestNoteListEmptyReturnsArray(t *testing.T) {
	f := newNoteFixture(t)
	w := httptest.NewRecorder()
	f.h.ListForParticipant(w, f.req(t, http.MethodGet, "/api/participants/1/notes", itoa(f.participantID), ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", w.Code)
	}
	if got := w.Body.String(); got != "[]\n" {
		t.Fatalf("empty list: want %q got %q", "[]\n", got)
	}
}

func TestNoteListReturnsCreated(t *testing.T) {
	f := newNoteFixture(t)
	body, _ := json.Marshal(map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-05", "body": "Entry.",
	})
	created := f.createNote(t, string(body))

	w := httptest.NewRecorder()
	f.h.ListForParticipant(w, f.req(t, http.MethodGet, "/api/participants/1/notes", itoa(f.participantID), ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d", w.Code)
	}
	var out []repository.Note
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 || out[0].ID != created.ID {
		t.Fatalf("list: want [%d] got %+v", created.ID, out)
	}
}

func TestNoteGetUnknown404(t *testing.T) {
	f := newNoteFixture(t)
	w := httptest.NewRecorder()
	f.h.Get(w, f.req(t, http.MethodGet, "/api/notes/99999", "99999", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("get unknown: want 404 got %d", w.Code)
	}
}

func TestNoteUpdateUnknown404(t *testing.T) {
	f := newNoteFixture(t)
	body, _ := json.Marshal(map[string]any{"serviceDate": "2026-01-06", "body": "Nope."})
	w := httptest.NewRecorder()
	f.h.Update(w, f.req(t, http.MethodPut, "/api/notes/99999", "99999", string(body)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("update unknown: want 404 got %d", w.Code)
	}
}

func TestNoteCreateEmptyBody400(t *testing.T) {
	f := newNoteFixture(t)
	body, _ := json.Marshal(map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-05", "body": "",
	})
	w := httptest.NewRecorder()
	f.h.Create(w, f.req(t, http.MethodPost, "/api/notes", "", string(body)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty body: want 400 got %d", w.Code)
	}
}

func TestNoteDelete204(t *testing.T) {
	f := newNoteFixture(t)
	body, _ := json.Marshal(map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-05", "body": "Entry.",
	})
	created := f.createNote(t, string(body))

	w := httptest.NewRecorder()
	f.h.Delete(w, f.req(t, http.MethodDelete, "/api/notes/1", itoa(created.ID), ""))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", w.Code)
	}
}

func TestNoteBillLinksNotes(t *testing.T) {
	f := newNoteFixture(t)
	body, _ := json.Marshal(map[string]any{
		"participantId": f.participantID, "serviceDate": "2026-01-05", "body": "Billable.",
	})
	created := f.createNote(t, string(body))

	billBody, _ := json.Marshal(map[string]any{
		"invoiceId": f.invoiceID, "noteIds": []int64{created.ID},
	})
	w := httptest.NewRecorder()
	f.h.Bill(w, f.req(t, http.MethodPost, "/api/notes/bill", "", string(billBody)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("bill: want 204 got %d (%s)", w.Code, w.Body.String())
	}

	lw := httptest.NewRecorder()
	f.h.ListForParticipant(lw, f.req(t, http.MethodGet, "/api/participants/1/notes", itoa(f.participantID), ""))
	if lw.Code != http.StatusOK {
		t.Fatalf("list after bill: want 200 got %d", lw.Code)
	}
	var out []repository.Note
	if err := json.NewDecoder(lw.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("list after bill: want 1 note got %d", len(out))
	}
	if out[0].BilledID == nil || *out[0].BilledID != f.invoiceID {
		t.Fatalf("billedInvoiceId: want %d got %v", f.invoiceID, out[0].BilledID)
	}
}
