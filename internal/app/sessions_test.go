package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/catalogue"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/session"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// sessionFixture bundles a migrated DB, the session handler mounted on a real test
// server, the seeded tenant context, and the seeded client uuid. Entities
// are addressed by uuid in the path + JSON, matching the production contract.
type sessionFixture struct {
	srv               *httptest.Server
	ctx               context.Context
	tenantID          string
	clientUUID        string
	catalogueItemUUID string
}

// newSessionFixture migrates a temp DB, seeds a tenant+client, and mounts the
// session routes behind a tenant-injecting middleware on a real test server.
func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	conn := openMigratedDB(t, "session.db")
	_, tenantID, _, _ := seedTenantOwner(t, conn)

	ctx := reqctx.WithTenant(context.Background(), tenantID)

	partSvc := client.NewService(conn)
	part, err := partSvc.Create(ctx, client.ClientInput{Name: "Stark"})
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if part == nil || part.ID == "" {
		t.Fatalf("seed client: want uuid got %+v", part)
	}

	ciSvc := catalogue.NewService(conn)
	ci, err := ciSvc.Create(ctx, catalogue.CatalogueItemInput{Name: "Mileage", UnitPrice: 0.85, Unit: "km"})
	if err != nil {
		t.Fatalf("seed custom item: %v", err)
	}
	if ci == nil || ci.ID == "" {
		t.Fatalf("seed custom item: want uuid got %+v", ci)
	}

	h := session.NewHandler(session.NewService(conn, invoice.NewInvoices(conn)))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	h.Routes(r)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &sessionFixture{
		srv:               srv,
		ctx:               ctx,
		tenantID:          tenantID,
		clientUUID:        part.ID,
		catalogueItemUUID: ci.ID,
	}
}

// do issues a request against the mounted session router and returns the response.
func (f *sessionFixture) do(t *testing.T, method, path, body string) *http.Response {
	t.Helper()
	var r *http.Request
	var err error
	if body == "" {
		r, err = http.NewRequest(method, f.srv.URL+path, nil)
	} else {
		r, err = http.NewRequest(method, f.srv.URL+path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, path, err)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	return resp
}

// createSession posts a session via the router and returns the decoded session.
func (f *sessionFixture) createSession(t *testing.T, body string) session.Session {
	t.Helper()
	resp := f.do(t, http.MethodPost, "/sessions", body)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("create session: want 201 got %d (%s)", resp.StatusCode, buf.String())
	}
	var out session.Session
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return out
}

func TestSessionCreateRoundTripsFields(t *testing.T) {
	f := newSessionFixture(t)
	body, err := json.Marshal(map[string]any{
		"clientId":    f.clientUUID,
		"serviceDate": "2026-01-05",
		"note":        "Took client shopping.",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := f.createSession(t, string(body))
	if s.ID == "" {
		t.Fatalf("create: want non-empty uuid got %q", s.ID)
	}
	if s.ClientUUID != f.clientUUID {
		t.Fatalf("clientId: want %q got %q", f.clientUUID, s.ClientUUID)
	}
	if s.ServiceDate != "2026-01-05" {
		t.Fatalf("serviceDate: want 2026-01-05 got %q", s.ServiceDate)
	}
	if s.Status != "recorded" {
		t.Fatalf("status: want recorded got %q", s.Status)
	}
}

func TestSessionCreateMissingServiceDate400(t *testing.T) {
	f := newSessionFixture(t)
	body, _ := json.Marshal(map[string]any{"clientId": f.clientUUID})
	resp := f.do(t, http.MethodPost, "/sessions", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing service date: want 400 got %d", resp.StatusCode)
	}
}

func TestSessionListForClientEmptyReturnsArray(t *testing.T) {
	f := newSessionFixture(t)
	resp := f.do(t, http.MethodGet, "/sessions?client="+f.clientUUID, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", resp.StatusCode)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if got := buf.String(); got != "[]\n" {
		t.Fatalf("empty list: want %q got %q", "[]\n", got)
	}
}

func TestSessionListForClientReturnsCreated(t *testing.T) {
	f := newSessionFixture(t)
	body, _ := json.Marshal(map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05", "note": "Entry.",
	})
	created := f.createSession(t, string(body))

	resp := f.do(t, http.MethodGet, "/sessions?client="+f.clientUUID, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", resp.StatusCode)
	}
	var out []session.Session
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 || out[0].ID != created.ID {
		t.Fatalf("list: want [%s] got %+v", created.ID, out)
	}
}

func TestSessionListForClientStatusFilter(t *testing.T) {
	f := newSessionFixture(t)
	rec := f.createSession(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-10", "status": "recorded",
	}))
	sched := f.createSession(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-11", "status": "scheduled",
	}))
	_ = sched

	resp := f.do(t, http.MethodGet, "/sessions?client="+f.clientUUID+"&status=recorded", "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status: want 200 got %d", resp.StatusCode)
	}
	var out []session.Session
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 || out[0].ID != rec.ID {
		t.Fatalf("status filter: want [%s] got %+v", rec.ID, out)
	}
}

func TestSessionListAllTenant(t *testing.T) {
	f := newSessionFixture(t)
	_ = f.createSession(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-10",
	}))
	_ = f.createSession(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-02-20",
	}))

	resp := f.do(t, http.MethodGet, "/sessions", "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list all: want 200 got %d", resp.StatusCode)
	}
	var out []session.Session
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("list all: want 2 got %d", len(out))
	}
}

func TestSessionGetUnknown404(t *testing.T) {
	f := newSessionFixture(t)
	resp := f.do(t, http.MethodGet, "/sessions/"+uuidpkg.NewString(), "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get unknown: want 404 got %d", resp.StatusCode)
	}
}

func TestSessionUpdateUnknown404(t *testing.T) {
	f := newSessionFixture(t)
	body, _ := json.Marshal(map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-06", "note": "Nope.",
	})
	resp := f.do(t, http.MethodPut, "/sessions/"+uuidpkg.NewString(), string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update unknown: want 404 got %d", resp.StatusCode)
	}
}

func TestSessionStatusTransition(t *testing.T) {
	f := newSessionFixture(t)
	created := f.createSession(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05", "status": "scheduled",
	}))

	resp := f.do(t, http.MethodPost, "/sessions/"+created.ID+"/status",
		mustJSON(t, map[string]any{"status": "recorded"}))
	if resp.StatusCode != http.StatusNoContent {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("status transition: want 204 got %d (%s)", resp.StatusCode, buf.String())
	}
	_ = resp.Body.Close()

	gr := f.do(t, http.MethodGet, "/sessions/"+created.ID, "")
	defer func() { _ = gr.Body.Close() }()
	if gr.StatusCode != http.StatusOK {
		t.Fatalf("get after status: want 200 got %d", gr.StatusCode)
	}
	var got session.Session
	if err := json.NewDecoder(gr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "recorded" {
		t.Fatalf("status: want recorded got %q", got.Status)
	}
}

func TestSessionStatusEmpty400(t *testing.T) {
	f := newSessionFixture(t)
	created := f.createSession(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05",
	}))
	resp := f.do(t, http.MethodPost, "/sessions/"+created.ID+"/status",
		mustJSON(t, map[string]any{"status": ""}))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty status: want 400 got %d", resp.StatusCode)
	}
}

func TestSessionDelete204(t *testing.T) {
	f := newSessionFixture(t)
	created := f.createSession(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05",
	}))

	resp := f.do(t, http.MethodDelete, "/sessions/"+created.ID, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestSessionSuggestionsAndToRecordEmpty(t *testing.T) {
	f := newSessionFixture(t)

	sw := f.do(t, http.MethodGet, "/sessions/suggestions", "")
	defer func() { _ = sw.Body.Close() }()
	if sw.StatusCode != http.StatusOK {
		t.Fatalf("suggestions: want 200 got %d", sw.StatusCode)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(sw.Body)
	if got := buf.String(); got != "[]\n" {
		t.Fatalf("suggestions empty: want %q got %q", "[]\n", got)
	}

	tw := f.do(t, http.MethodGet, "/sessions/to-record", "")
	defer func() { _ = tw.Body.Close() }()
	if tw.StatusCode != http.StatusOK {
		t.Fatalf("to-record: want 200 got %d", tw.StatusCode)
	}
	tbuf := new(bytes.Buffer)
	_, _ = tbuf.ReadFrom(tw.Body)
	if got := tbuf.String(); got != "[]\n" {
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

// TestSessionItemCustomItemRoundTrips verifies a custom-item uuid set on a session
// line item round-trips on GET; an unknown custom-item uuid is rejected.
func TestSessionItemCustomItemRoundTrips(t *testing.T) {
	f := newSessionFixture(t)
	sessionBody, err := json.Marshal(map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05", "note": "x",
	})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	s := f.createSession(t, string(sessionBody))

	itemBody, err := json.Marshal(map[string]any{
		"description": "Trip", "quantity": 3, "unitPrice": 0.85, "catalogueItemId": f.catalogueItemUUID,
	})
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	resp := f.do(t, http.MethodPost, "/sessions/"+s.ID+"/items", string(itemBody))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("add item: want 201 got %d (%s)", resp.StatusCode, buf.String())
	}
	var item struct {
		ID              string  `json:"id"`
		CatalogueItemID *string `json:"catalogueItemId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode item: %v", err)
	}
	if item.CatalogueItemID == nil || *item.CatalogueItemID != f.catalogueItemUUID {
		t.Fatalf("add customItemId: want %q got %v", f.catalogueItemUUID, item.CatalogueItemID)
	}

	listResp := f.do(t, http.MethodGet, "/sessions/"+s.ID+"/items", "")
	defer func() { _ = listResp.Body.Close() }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list items: want 200 got %d", listResp.StatusCode)
	}
	var items []struct {
		CatalogueItemID *string `json:"catalogueItemId"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0].CatalogueItemID == nil || *items[0].CatalogueItemID != f.catalogueItemUUID {
		t.Fatalf("list customItemId: want %q got %v", f.catalogueItemUUID, items)
	}
}

// TestSessionItemUnknownCatalogueItem422 verifies an unknown catalogue-item uuid
// on a session item add is rejected by the line validator (422).
func TestSessionItemUnknownCatalogueItem422(t *testing.T) {
	f := newSessionFixture(t)
	sessionBody, err := json.Marshal(map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05", "note": "x",
	})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	s := f.createSession(t, string(sessionBody))

	itemBody, err := json.Marshal(map[string]any{
		"description": "Trip", "quantity": 1, "unitPrice": 5, "catalogueItemId": uuidpkg.NewString(),
	})
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	resp := f.do(t, http.MethodPost, "/sessions/"+s.ID+"/items", string(itemBody))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown catalogue item: want 422 got %d", resp.StatusCode)
	}
}
