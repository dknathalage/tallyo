package recurring

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// mountRecurring returns a router with the slice routes mounted and a middleware
// that attaches the tenant id to every request (standing in for auth).
func mountRecurring(h *Handler, tenantID int64) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	h.Routes(r)
	return r
}

// newRecurringHandler builds a handler over a fresh DB with a seeded
// tenant+participant+template, returning the handler, tenant id, participant
// uuid, and the seeded template.
func newRecurringHandler(t *testing.T) (*Handler, int64, string, *RecurringTemplate) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	participantUUID := seedParticipant(t, conn, tenantID, "Jane")
	repo := NewRepo(conn)
	tpl := mkTemplate(t, repo, tenantID, participantUUID, "2026-01-01")
	svc := NewService(conn, realtime.NewHub())
	return NewHandler(svc), tenantID, participantUUID, tpl
}

func TestRecurringGetByUUID(t *testing.T) {
	h, tenantID, participantUUID, tpl := newRecurringHandler(t)
	srv := httptest.NewServer(mountRecurring(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/recurring/" + tpl.UUID)
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
	if got["id"] != tpl.UUID {
		t.Fatalf("json id=%v want uuid %q", got["id"], tpl.UUID)
	}
	if got["participantId"] != participantUUID {
		t.Fatalf("json participantId=%v want participant uuid %q", got["participantId"], participantUUID)
	}
}

func TestRecurringGetUnknownUUID404(t *testing.T) {
	h, tenantID, _, _ := newRecurringHandler(t)
	srv := httptest.NewServer(mountRecurring(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/recurring/3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", res.StatusCode)
	}
}

func TestRecurringGetNonUUID400(t *testing.T) {
	h, tenantID, _, _ := newRecurringHandler(t)
	srv := httptest.NewServer(mountRecurring(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/recurring/123")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", res.StatusCode)
	}
}

// TestRecurringCreateResolvesParticipantUUID proves an inbound participantId uuid
// resolves to the FK and round-trips back as the same uuid; an unknown
// participant uuid is rejected with 400.
func TestRecurringCreateResolvesParticipantUUID(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	participantUUID := seedParticipant(t, conn, tenantID, "Jane")
	h := NewHandler(NewService(conn, realtime.NewHub()))
	srv := httptest.NewServer(mountRecurring(h, tenantID))
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"name":          "Weekly",
		"participantId": participantUUID,
		"frequency":     "weekly",
		"nextDue":       "2026-01-01",
		"lineItems":     []map[string]any{{"description": "Support", "quantity": 1, "unitPrice": 100}},
		"isActive":      true,
	})
	res, err := http.Post(srv.URL+"/recurring", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d want 201", res.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created["participantId"] != participantUUID {
		t.Fatalf("created participantId=%v want %q", created["participantId"], participantUUID)
	}

	// An unknown participant uuid is rejected with 400.
	badBody, _ := json.Marshal(map[string]any{
		"name":          "Bad",
		"participantId": "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c",
		"frequency":     "weekly",
		"nextDue":       "2026-01-01",
	})
	badRes, err := http.Post(srv.URL+"/recurring", "application/json", bytes.NewReader(badBody))
	if err != nil {
		t.Fatalf("POST bad: %v", err)
	}
	defer badRes.Body.Close()
	if badRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown participant uuid status=%d want 400", badRes.StatusCode)
	}
}

// TestRecurringGenerateByUUID proves POST /recurring/{uuid}/generate produces a
// draft invoice (returned with 200) addressed by the template uuid.
func TestRecurringGenerateByUUID(t *testing.T) {
	h, tenantID, _, tpl := newRecurringHandler(t)
	srv := httptest.NewServer(mountRecurring(h, tenantID))
	defer srv.Close()

	res, err := http.Post(srv.URL+"/recurring/"+tpl.UUID+"/generate", "application/json", nil)
	if err != nil {
		t.Fatalf("POST generate: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("generate status=%d want 200", res.StatusCode)
	}
	var inv map[string]any
	if err := json.NewDecoder(res.Body).Decode(&inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inv["number"] != "INV-0001" {
		t.Fatalf("generated invoice number=%v want INV-0001", inv["number"])
	}
}
