package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"
)

// newCatalogIngestServer wires the platform-admin catalogue ingest route behind
// RequireAuth + RequirePlatformAdmin. seedTenantOwner creates the "o@x.com"
// owner as a platform admin; we additionally seed a non-admin "member" so the
// 403 path is exercised.
func newCatalogIngestServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn := openMigratedDB(t, "catalog_ingest.db")
	users, tenantID, _ := seedTenantOwner(t, conn)

	// A non-platform-admin tenant member.
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := users.Create(t.Context(), tenantID, "member@x.com", hash, "", "member", false); err != nil {
		t.Fatalf("Create member: %v", err)
	}

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, auth.NewTenants(conn))
	scH := catalog.NewHandler(
		catalog.NewService(conn),
		catalog.NewIngestService(conn, hub),
	)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users, auth.NewTenants(conn)))
			pr.Get("/support-catalog/versions", scH.ListVersions)
			pr.With(RequirePlatformAdmin).Post("/support-catalog/versions", scH.Ingest)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// catalogXLSXBytes builds a minimal NDIS-Support-Catalogue-shaped XLSX.
func catalogXLSXBytes(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Sheet1"
	_ = f.SetSheetRow(sheet, "A1", &[]any{
		"Support Item Number", "Support Item Name", "Unit",
		"Support Category", "Registration Group Name",
		"National", "Remote", "Very Remote",
	})
	_ = f.SetSheetRow(sheet, "A2", &[]any{
		"01_011_0107_1_1", "Assistance With Self-Care", "Hour",
		"Core", "Daily Living", "$67.56", "$94.58", "$101.34",
	})
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}
	return buf.Bytes()
}

// uploadCatalog posts a multipart catalogue upload and returns the response.
func uploadCatalog(t *testing.T, c *http.Client, base string, xlsx []byte, label, effectiveFrom string) *http.Response {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if label != "" {
		_ = w.WriteField("label", label)
	}
	if effectiveFrom != "" {
		_ = w.WriteField("effectiveFrom", effectiveFrom)
	}
	if xlsx != nil {
		fw, err := w.CreateFormFile("file", "catalogue.xlsx")
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write(xlsx); err != nil {
			t.Fatalf("write file part: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req, err := http.NewRequest("POST", base+"/api/support-catalog/versions", &body)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func TestCatalogIngestPlatformAdminSucceeds(t *testing.T) {
	srv := newCatalogIngestServer(t)
	c := loggedInClient(t, srv.URL) // o@x.com is a platform admin
	resp := uploadCatalog(t, c, srv.URL, catalogXLSXBytes(t), "2025-26 v1.1", "2025-07-01")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("admin upload: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		VersionID  int64 `json:"versionId"`
		ItemCount  int   `json:"itemCount"`
		PriceCount int   `json:"priceCount"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.VersionID <= 0 || out.ItemCount != 1 || out.PriceCount != 3 {
		t.Fatalf("summary = %+v, want versionId>0 itemCount=1 priceCount=3", out)
	}
}

func TestCatalogIngestNonAdminForbidden(t *testing.T) {
	srv := newCatalogIngestServer(t)
	c := jarClient(t)
	lr := login(t, c, srv.URL, "member@x.com", "password1")
	_ = lr.Body.Close()
	if lr.StatusCode != http.StatusOK {
		t.Fatalf("member login: want 200 got %d", lr.StatusCode)
	}
	resp := uploadCatalog(t, c, srv.URL, catalogXLSXBytes(t), "x", "2025-07-01")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin upload: want 403 got %d", resp.StatusCode)
	}
}

func TestCatalogIngestUnauthenticated401(t *testing.T) {
	srv := newCatalogIngestServer(t)
	c := jarClient(t)
	resp := uploadCatalog(t, c, srv.URL, catalogXLSXBytes(t), "x", "2025-07-01")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon upload: want 401 got %d", resp.StatusCode)
	}
}

func TestCatalogIngestMissingFieldsRejected(t *testing.T) {
	srv := newCatalogIngestServer(t)
	c := loggedInClient(t, srv.URL)
	resp := uploadCatalog(t, c, srv.URL, catalogXLSXBytes(t), "", "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing fields: want 400 got %d", resp.StatusCode)
	}
}
