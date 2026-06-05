package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newExportServer wires the export routes behind RequireAuth and seeds two
// catalog items so the catalog exports are non-empty.
func newExportServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "export.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	users := auth.NewUsers(conn)
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := users.Create(t.Context(), "o@x.com", hash, "owner"); err != nil {
		t.Fatalf("Create owner: %v", err)
	}

	hub := realtime.NewHub()
	catalogSvc := service.NewCatalogService(conn, hub)
	if _, err := catalogSvc.Create(t.Context(), repository.CatalogItemInput{Name: "Consulting", Rate: 150.5, Unit: "hour", Category: "Services", Sku: "CON-1"}); err != nil {
		t.Fatalf("seed item 1: %v", err)
	}
	if _, err := catalogSvc.Create(t.Context(), repository.CatalogItemInput{Name: "Design", Rate: 90, Unit: "hour", Category: "Services", Sku: "DES-2"}); err != nil {
		t.Fatalf("seed item 2: %v", err)
	}

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	expH := NewExportHandler(
		catalogSvc,
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
	if !strings.HasPrefix(string(b), "name,sku,rate,unit,category") {
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
	if !strings.HasPrefix(string(b), "invoiceNumber,clientName,date,dueDate,status,subtotal,taxAmount,total,currency") {
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
	if !strings.HasPrefix(string(b), "estimateNumber,clientName,date,validUntil,status,subtotal,taxAmount,total,currency") {
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
