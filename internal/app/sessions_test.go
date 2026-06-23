package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/customitem"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// shiftFixture bundles a migrated DB, the shift handler mounted on a real test
// server, the seeded tenant context, and the seeded client uuid. Entities
// are addressed by uuid in the path + JSON, matching the production contract.
type shiftFixture struct {
	srv            *httptest.Server
	ctx            context.Context
	tenantID       int64
	clientUUID     string
	customItemUUID string
}

// newShiftFixture migrates a temp DB, seeds a tenant+client, and mounts the
// shift routes behind a tenant-injecting middleware on a real test server.
func newShiftFixture(t *testing.T) *shiftFixture {
	t.Helper()
	conn := openMigratedDB(t, "shift.db")
	_, tenantID, _, _ := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	ctx := reqctx.WithTenant(context.Background(), tenantID)

	partSvc := client.NewService(conn, hub)
	part, err := partSvc.Create(ctx, client.ClientInput{Name: "Stark"})
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if part == nil || part.UUID == "" {
		t.Fatalf("seed client: want uuid got %+v", part)
	}

	ciSvc := customitem.NewService(conn, hub)
	ci, err := ciSvc.Create(ctx, customitem.CustomItemInput{Name: "Mileage", Rate: 0.85, Unit: "km"})
	if err != nil {
		t.Fatalf("seed custom item: %v", err)
	}
	if ci == nil || ci.UUID == "" {
		t.Fatalf("seed custom item: want uuid got %+v", ci)
	}

	h := shift.NewHandler(shift.NewService(conn, conn, hub, invoice.NewInvoices(conn)), nil)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	h.Routes(r)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &shiftFixture{
		srv:            srv,
		ctx:            ctx,
		tenantID:       tenantID,
		clientUUID:     part.UUID,
		customItemUUID: ci.UUID,
	}
}

// do issues a request against the mounted shift router and returns the response.
func (f *shiftFixture) do(t *testing.T, method, path, body string) *http.Response {
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

// createShift posts a shift via the router and returns the decoded shift.
func (f *shiftFixture) createShift(t *testing.T, body string) shift.Shift {
	t.Helper()
	resp := f.do(t, http.MethodPost, "/shifts", body)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("create shift: want 201 got %d (%s)", resp.StatusCode, buf.String())
	}
	var out shift.Shift
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode shift: %v", err)
	}
	return out
}

func TestShiftCreateRoundTripsFields(t *testing.T) {
	f := newShiftFixture(t)
	body, err := json.Marshal(map[string]any{
		"clientId":    f.clientUUID,
		"serviceDate": "2026-01-05",
		"note":        "Took client shopping.",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := f.createShift(t, string(body))
	if s.UUID == "" {
		t.Fatalf("create: want non-empty uuid got %q", s.UUID)
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

func TestShiftCreateMissingServiceDate400(t *testing.T) {
	f := newShiftFixture(t)
	body, _ := json.Marshal(map[string]any{"clientId": f.clientUUID})
	resp := f.do(t, http.MethodPost, "/shifts", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing service date: want 400 got %d", resp.StatusCode)
	}
}

func TestShiftListForClientEmptyReturnsArray(t *testing.T) {
	f := newShiftFixture(t)
	resp := f.do(t, http.MethodGet, "/shifts?client="+f.clientUUID, "")
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

func TestShiftListForClientReturnsCreated(t *testing.T) {
	f := newShiftFixture(t)
	body, _ := json.Marshal(map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05", "note": "Entry.",
	})
	created := f.createShift(t, string(body))

	resp := f.do(t, http.MethodGet, "/shifts?client="+f.clientUUID, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", resp.StatusCode)
	}
	var out []shift.Shift
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 || out[0].UUID != created.UUID {
		t.Fatalf("list: want [%s] got %+v", created.UUID, out)
	}
}

func TestShiftListForClientStatusFilter(t *testing.T) {
	f := newShiftFixture(t)
	rec := f.createShift(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-10", "status": "recorded",
	}))
	sched := f.createShift(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-11", "status": "scheduled",
	}))
	_ = sched

	resp := f.do(t, http.MethodGet, "/shifts?client="+f.clientUUID+"&status=recorded", "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status: want 200 got %d", resp.StatusCode)
	}
	var out []shift.Shift
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 || out[0].UUID != rec.UUID {
		t.Fatalf("status filter: want [%s] got %+v", rec.UUID, out)
	}
}

func TestShiftListAllTenant(t *testing.T) {
	f := newShiftFixture(t)
	_ = f.createShift(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-10",
	}))
	_ = f.createShift(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-02-20",
	}))

	resp := f.do(t, http.MethodGet, "/shifts", "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list all: want 200 got %d", resp.StatusCode)
	}
	var out []shift.Shift
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("list all: want 2 got %d", len(out))
	}
}

func TestShiftGetUnknown404(t *testing.T) {
	f := newShiftFixture(t)
	resp := f.do(t, http.MethodGet, "/shifts/"+uuidpkg.NewString(), "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get unknown: want 404 got %d", resp.StatusCode)
	}
}

func TestShiftUpdateUnknown404(t *testing.T) {
	f := newShiftFixture(t)
	body, _ := json.Marshal(map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-06", "note": "Nope.",
	})
	resp := f.do(t, http.MethodPut, "/shifts/"+uuidpkg.NewString(), string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update unknown: want 404 got %d", resp.StatusCode)
	}
}

func TestShiftStatusTransition(t *testing.T) {
	f := newShiftFixture(t)
	created := f.createShift(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05", "status": "scheduled",
	}))

	resp := f.do(t, http.MethodPost, "/shifts/"+created.UUID+"/status",
		mustJSON(t, map[string]any{"status": "recorded"}))
	if resp.StatusCode != http.StatusNoContent {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("status transition: want 204 got %d (%s)", resp.StatusCode, buf.String())
	}
	_ = resp.Body.Close()

	gr := f.do(t, http.MethodGet, "/shifts/"+created.UUID, "")
	defer func() { _ = gr.Body.Close() }()
	if gr.StatusCode != http.StatusOK {
		t.Fatalf("get after status: want 200 got %d", gr.StatusCode)
	}
	var got shift.Shift
	if err := json.NewDecoder(gr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "recorded" {
		t.Fatalf("status: want recorded got %q", got.Status)
	}
}

func TestShiftStatusEmpty400(t *testing.T) {
	f := newShiftFixture(t)
	created := f.createShift(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05",
	}))
	resp := f.do(t, http.MethodPost, "/shifts/"+created.UUID+"/status",
		mustJSON(t, map[string]any{"status": ""}))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty status: want 400 got %d", resp.StatusCode)
	}
}

func TestShiftDelete204(t *testing.T) {
	f := newShiftFixture(t)
	created := f.createShift(t, mustJSON(t, map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05",
	}))

	resp := f.do(t, http.MethodDelete, "/shifts/"+created.UUID, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestShiftSuggestionsAndToRecordEmpty(t *testing.T) {
	f := newShiftFixture(t)

	sw := f.do(t, http.MethodGet, "/shifts/suggestions", "")
	defer func() { _ = sw.Body.Close() }()
	if sw.StatusCode != http.StatusOK {
		t.Fatalf("suggestions: want 200 got %d", sw.StatusCode)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(sw.Body)
	if got := buf.String(); got != "[]\n" {
		t.Fatalf("suggestions empty: want %q got %q", "[]\n", got)
	}

	tw := f.do(t, http.MethodGet, "/shifts/to-record", "")
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

// TestShiftItemCustomItemRoundTrips verifies a custom-item uuid set on a shift
// line item round-trips on GET; an unknown custom-item uuid is rejected.
func TestShiftItemCustomItemRoundTrips(t *testing.T) {
	f := newShiftFixture(t)
	shiftBody, err := json.Marshal(map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05", "note": "x",
	})
	if err != nil {
		t.Fatalf("marshal shift: %v", err)
	}
	s := f.createShift(t, string(shiftBody))

	itemBody, err := json.Marshal(map[string]any{
		"description": "Trip", "quantity": 3, "unitPrice": 0.85, "customItemId": f.customItemUUID,
	})
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	resp := f.do(t, http.MethodPost, "/shifts/"+s.UUID+"/items", string(itemBody))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("add item: want 201 got %d (%s)", resp.StatusCode, buf.String())
	}
	var item struct {
		ID           string  `json:"id"`
		CustomItemID *string `json:"customItemId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode item: %v", err)
	}
	if item.CustomItemID == nil || *item.CustomItemID != f.customItemUUID {
		t.Fatalf("add customItemId: want %q got %v", f.customItemUUID, item.CustomItemID)
	}

	listResp := f.do(t, http.MethodGet, "/shifts/"+s.UUID+"/items", "")
	defer func() { _ = listResp.Body.Close() }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list items: want 200 got %d", listResp.StatusCode)
	}
	var items []struct {
		CustomItemID *string `json:"customItemId"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0].CustomItemID == nil || *items[0].CustomItemID != f.customItemUUID {
		t.Fatalf("list customItemId: want %q got %v", f.customItemUUID, items)
	}
}

// TestShiftItemUnknownCustomItem400 verifies an unknown custom-item uuid on a
// shift item add is rejected.
func TestShiftItemUnknownCustomItem400(t *testing.T) {
	f := newShiftFixture(t)
	shiftBody, err := json.Marshal(map[string]any{
		"clientId": f.clientUUID, "serviceDate": "2026-01-05", "note": "x",
	})
	if err != nil {
		t.Fatalf("marshal shift: %v", err)
	}
	s := f.createShift(t, string(shiftBody))

	itemBody, err := json.Marshal(map[string]any{
		"description": "Trip", "quantity": 1, "unitPrice": 5, "customItemId": uuidpkg.NewString(),
	})
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	resp := f.do(t, http.MethodPost, "/shifts/"+s.UUID+"/items", string(itemBody))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown custom item: want 400 got %d", resp.StatusCode)
	}
}
