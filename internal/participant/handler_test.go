package participant

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/planmanager"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// newParticipantHandler builds a handler over a fresh DB and returns it with the
// tenant id, a seeded plan manager (its uuid), and a participant seeded WITH that
// plan manager.
func newParticipantHandler(t *testing.T) (*Handler, int64, string, *Participant) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	pm, err := planmanager.NewPlanManagers(conn).Create(tctx(tenantID), tenantID, planmanager.PlanManagerInput{Name: "PM Co"})
	if err != nil {
		t.Fatalf("seed plan manager: %v", err)
	}
	svc := NewService(conn, realtime.NewHub())
	seeded, err := svc.Create(tctx(tenantID), ParticipantInput{Name: "Jane", PlanManagerUUID: &pm.UUID})
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}
	return NewHandler(svc), tenantID, pm.UUID, seeded
}

// mountParticipant returns a router with the slice routes mounted and a
// middleware that attaches the tenant id to every request (standing in for auth).
func mountParticipant(h *Handler, tenantID int64) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	h.Routes(r)
	return r
}

func TestParticipantGetByUUID(t *testing.T) {
	h, tenantID, pmUUID, seeded := newParticipantHandler(t)
	srv := httptest.NewServer(mountParticipant(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/participants/" + seeded.UUID)
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
	if got["id"] != seeded.UUID {
		t.Fatalf("json id=%v want uuid %q", got["id"], seeded.UUID)
	}
	if got["planManagerId"] != pmUUID {
		t.Fatalf("json planManagerId=%v want plan-manager uuid %q", got["planManagerId"], pmUUID)
	}
}

func TestParticipantGetUnknownUUID404(t *testing.T) {
	h, tenantID, _, _ := newParticipantHandler(t)
	srv := httptest.NewServer(mountParticipant(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/participants/3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", res.StatusCode)
	}
}

func TestParticipantGetNonUUID400(t *testing.T) {
	h, tenantID, _, _ := newParticipantHandler(t)
	srv := httptest.NewServer(mountParticipant(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/participants/123")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", res.StatusCode)
	}
}

// TestParticipantCreateResolvesPlanManagerUUID proves an inbound planManagerId
// uuid resolves to the FK and round-trips back as the same uuid; an unknown
// plan-manager uuid is rejected with 400.
func TestParticipantCreateResolvesPlanManagerUUID(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	pm, err := planmanager.NewPlanManagers(conn).Create(tctx(tenantID), tenantID, planmanager.PlanManagerInput{Name: "PM Co"})
	if err != nil {
		t.Fatalf("seed plan manager: %v", err)
	}
	h := NewHandler(NewService(conn, realtime.NewHub()))
	srv := httptest.NewServer(mountParticipant(h, tenantID))
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"name": "Jane", "planManagerId": pm.UUID})
	res, err := http.Post(srv.URL+"/participants", "application/json", bytes.NewReader(body))
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
	if created["planManagerId"] != pm.UUID {
		t.Fatalf("created planManagerId=%v want %q", created["planManagerId"], pm.UUID)
	}

	// Update to clear the plan manager (empty string → NULL FK).
	createdUUID, _ := created["id"].(string)
	upBody, _ := json.Marshal(map[string]any{"name": "Jane", "planManagerId": nil})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/participants/"+createdUUID, bytes.NewReader(upBody))
	req.Header.Set("Content-Type", "application/json")
	upRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer upRes.Body.Close()
	if upRes.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d want 200", upRes.StatusCode)
	}
	var updated map[string]any
	if err := json.NewDecoder(upRes.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updated["planManagerId"] != nil {
		t.Fatalf("updated planManagerId=%v want nil", updated["planManagerId"])
	}

	// An unknown plan-manager uuid is rejected with 400.
	badBody, _ := json.Marshal(map[string]any{"name": "Bob", "planManagerId": "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c"})
	badRes, err := http.Post(srv.URL+"/participants", "application/json", bytes.NewReader(badBody))
	if err != nil {
		t.Fatalf("POST bad: %v", err)
	}
	defer badRes.Body.Close()
	if badRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown plan-manager uuid status=%d want 400", badRes.StatusCode)
	}
}
