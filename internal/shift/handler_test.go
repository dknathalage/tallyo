package shift

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// seedParticipantUUID inserts a participant and returns its (int id, uuid).
func seedParticipantUUID(t *testing.T, conn *sql.DB, tenantID int64, name string) (int64, string) {
	t.Helper()
	id := seedParticipant(t, conn, tenantID, name)
	row, err := gen.New(conn).GetParticipantByID(context.Background(), gen.GetParticipantByIDParams{TenantID: tenantID, ID: id})
	if err != nil {
		t.Fatalf("read participant uuid: %v", err)
	}
	return id, row.Uuid
}

// newShiftHandler builds a handler over a fresh DB, seeds a participant + one
// recorded shift, and returns the handler, tenant id, participant uuid, and the
// seeded shift.
func newShiftHandler(t *testing.T) (*Handler, int64, string, *Shift) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	_, pUUID := seedParticipantUUID(t, conn, tenantID, "Jane")
	svc := NewService(conn, conn, realtime.NewHub(), nil)
	pid := participantIDFor(t, conn, tenantID, pUUID)
	sh, err := svc.Create(tctx(tenantID), ShiftInput{ParticipantID: pid, ServiceDate: "2026-01-15", Note: "n"})
	if err != nil {
		t.Fatalf("seed shift: %v", err)
	}
	return NewHandler(svc, nil), tenantID, pUUID, sh
}

func participantIDFor(t *testing.T, conn *sql.DB, tenantID int64, pUUID string) int64 {
	t.Helper()
	id, err := gen.New(conn).GetParticipantIDByUUID(context.Background(), gen.GetParticipantIDByUUIDParams{TenantID: tenantID, Uuid: pUUID})
	if err != nil {
		t.Fatalf("resolve participant uuid: %v", err)
	}
	return id
}

func mountShift(h *Handler, tenantID int64) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	h.Routes(r)
	return r
}

func TestShiftGetByUUID(t *testing.T) {
	h, tenantID, pUUID, sh := newShiftHandler(t)
	srv := httptest.NewServer(mountShift(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/shifts/" + sh.UUID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["id"] != sh.UUID {
		t.Fatalf("json id=%v want shift uuid %q", got["id"], sh.UUID)
	}
	if got["participantId"] != pUUID {
		t.Fatalf("json participantId=%v want participant uuid %q", got["participantId"], pUUID)
	}
}

func TestShiftGetUnknownUUID404(t *testing.T) {
	h, tenantID, _, _ := newShiftHandler(t)
	srv := httptest.NewServer(mountShift(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/shifts/3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", res.StatusCode)
	}
}

func TestShiftGetNonUUID400(t *testing.T) {
	h, tenantID, _, _ := newShiftHandler(t)
	srv := httptest.NewServer(mountShift(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/shifts/123")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", res.StatusCode)
	}
}

// TestShiftItemLifecycleByUUID exercises POST/GET/PATCH/DELETE of a shift's line
// items addressed by item uuid under the shift uuid.
func TestShiftItemLifecycleByUUID(t *testing.T) {
	h, tenantID, _, sh := newShiftHandler(t)
	srv := httptest.NewServer(mountShift(h, tenantID))
	defer srv.Close()

	// POST a custom (non-catalogue) line.
	body, _ := json.Marshal(map[string]any{"description": "travel", "unit": "EA", "quantity": 2, "unitPrice": 1.5})
	res, err := http.Post(srv.URL+"/shifts/"+sh.UUID+"/items", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST item: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST item status=%d want 201", res.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode created item: %v", err)
	}
	itemUUID, _ := created["id"].(string)
	if itemUUID == "" {
		t.Fatalf("created item id not a uuid: %v", created["id"])
	}

	// GET the items list — must contain the item with id == item uuid.
	listRes, err := http.Get(srv.URL + "/shifts/" + sh.UUID + "/items")
	if err != nil {
		t.Fatalf("GET items: %v", err)
	}
	defer listRes.Body.Close()
	var items []map[string]any
	if err := json.NewDecoder(listRes.Body).Decode(&items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0]["id"] != itemUUID {
		t.Fatalf("items=%v want one with id %q", items, itemUUID)
	}

	// PATCH the item by uuid.
	upBody, _ := json.Marshal(map[string]any{"description": "travel", "unit": "EA", "quantity": 3, "unitPrice": 1.5})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/shifts/"+sh.UUID+"/items/"+itemUUID, bytes.NewReader(upBody))
	req.Header.Set("Content-Type", "application/json")
	upRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH item: %v", err)
	}
	defer upRes.Body.Close()
	if upRes.StatusCode != http.StatusOK {
		t.Fatalf("PATCH item status=%d want 200", upRes.StatusCode)
	}
	var patched map[string]any
	if err := json.NewDecoder(upRes.Body).Decode(&patched); err != nil {
		t.Fatalf("decode patched: %v", err)
	}
	if patched["id"] != itemUUID {
		t.Fatalf("patched id=%v want %q", patched["id"], itemUUID)
	}
	if patched["quantity"].(float64) != 3 {
		t.Fatalf("patched quantity=%v want 3", patched["quantity"])
	}

	// DELETE the item by uuid.
	delReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/shifts/"+sh.UUID+"/items/"+itemUUID, nil)
	delRes, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE item: %v", err)
	}
	defer delRes.Body.Close()
	if delRes.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE item status=%d want 204", delRes.StatusCode)
	}

	// List is now empty.
	finalRes, err := http.Get(srv.URL + "/shifts/" + sh.UUID + "/items")
	if err != nil {
		t.Fatalf("GET items after delete: %v", err)
	}
	defer finalRes.Body.Close()
	var after []map[string]any
	if err := json.NewDecoder(finalRes.Body).Decode(&after); err != nil {
		t.Fatalf("decode after: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("items after delete = %d, want 0", len(after))
	}
}

// TestShiftListByParticipantFilter proves GET /shifts?participant={uuid} filters
// to that participant's shifts (resolving the participant uuid→int internally).
func TestShiftListByParticipantFilter(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	_, p1UUID := seedParticipantUUID(t, conn, tenantID, "Jane")
	_, p2UUID := seedParticipantUUID(t, conn, tenantID, "Bob")
	svc := NewService(conn, conn, realtime.NewHub(), nil)
	p1 := participantIDFor(t, conn, tenantID, p1UUID)
	p2 := participantIDFor(t, conn, tenantID, p2UUID)
	ctx := tctx(tenantID)
	if _, err := svc.Create(ctx, ShiftInput{ParticipantID: p1, ServiceDate: "2026-01-10"}); err != nil {
		t.Fatalf("seed p1: %v", err)
	}
	if _, err := svc.Create(ctx, ShiftInput{ParticipantID: p1, ServiceDate: "2026-01-11"}); err != nil {
		t.Fatalf("seed p1b: %v", err)
	}
	if _, err := svc.Create(ctx, ShiftInput{ParticipantID: p2, ServiceDate: "2026-01-12"}); err != nil {
		t.Fatalf("seed p2: %v", err)
	}

	h := NewHandler(svc, nil)
	srv := httptest.NewServer(mountShift(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/shifts?participant=" + p1UUID)
	if err != nil {
		t.Fatalf("GET filtered: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	var got []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filtered shifts = %d, want 2", len(got))
	}
	for i := range got {
		if got[i]["participantId"] != p1UUID {
			t.Fatalf("shift participantId=%v want %q", got[i]["participantId"], p1UUID)
		}
	}
}
