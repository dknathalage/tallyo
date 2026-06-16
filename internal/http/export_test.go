package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newExportServer wires the export routes behind RequireAuth and seeds two
// custom items so the catalog exports are non-empty. Seeding goes through the
// authenticated owner's tenant context so the items belong to the tenant.
func newExportServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn := openMigratedDB(t, "export.db")
	users, tenantID, _ := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	customItemSvc := service.NewCustomItemService(conn, hub)
	seedCtx := reqctx.WithTenant(t.Context(), tenantID)
	if _, err := customItemSvc.Create(seedCtx, repository.CustomItemInput{Name: "Consulting", Rate: 150.5, Unit: "hour"}); err != nil {
		t.Fatalf("seed item 1: %v", err)
	}
	if _, err := customItemSvc.Create(seedCtx, repository.CustomItemInput{Name: "Design", Rate: 90, Unit: "hour"}); err != nil {
		t.Fatalf("seed item 2: %v", err)
	}

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	expH := NewExportHandler(
		customItemSvc,
		service.NewInvoiceService(conn, hub),
		service.NewEstimateService(conn, hub),
	)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/export/catalog", expH.Catalog)
			pr.Get("/export/invoices", expH.Invoices)
			pr.Get("/export/estimates", expH.Estimates)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return b
}

func TestExportCatalogCSV(t *testing.T) {
	srv := newExportServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/export/catalog")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/csv" {
		t.Fatalf("content-type: want text/csv got %q", ct)
	}
	b := readBody(t, resp)
	if !strings.HasPrefix(string(b), "name,rate,unit,gstFree") {
		t.Fatalf("missing CSV header: %q", b)
	}
	if !strings.Contains(string(b), "Consulting") {
		t.Fatalf("missing seeded item: %q", b)
	}
}

func TestExportCatalogXLSX(t *testing.T) {
	srv := newExportServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/export/catalog?format=xlsx")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("content-type: want spreadsheet got %q", ct)
	}
	b := readBody(t, resp)
	if !bytes.HasPrefix(b, []byte("PK")) {
		t.Fatalf("xlsx must start with PK, got %q", b[:min(2, len(b))])
	}
	if len(b) <= 500 {
		t.Fatalf("xlsx too small: %d", len(b))
	}
}

func TestExportInvoicesCSV(t *testing.T) {
	srv := newExportServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/export/invoices")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/csv" {
		t.Fatalf("content-type: want text/csv got %q", ct)
	}
	b := readBody(t, resp)
	if !strings.HasPrefix(string(b), "number,participantName,issueDate,dueDate,status,subtotal,tax,total") {
		t.Fatalf("missing CSV header: %q", b)
	}
}

func TestExportEstimatesCSV(t *testing.T) {
	srv := newExportServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/export/estimates")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", resp.StatusCode)
	}
	b := readBody(t, resp)
	if !strings.HasPrefix(string(b), "number,participantName,issueDate,validUntil,status,subtotal,tax,total") {
		t.Fatalf("missing CSV header: %q", b)
	}
}

func TestExportCatalogUnauthorized(t *testing.T) {
	srv := newExportServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/export/catalog")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", resp.StatusCode)
	}
}
