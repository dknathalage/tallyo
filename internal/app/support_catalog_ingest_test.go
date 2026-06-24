package app

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/pricelist"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/go-chi/chi/v5"
)

// newPriceListImportServer wires the per-tenant price-list import routes behind
// httpx.RequireSession + httpx.RequireRole("owner","admin"). seedTenantOwner
// creates the "o@x.com" owner; we additionally seed a non-owner/admin "member"
// so the 403 path is exercised.
func newPriceListImportServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "pricelist_import.db")
	users, tenantID, _, tenantUUID := seedTenantOwner(t, conn)

	// A non-owner/admin tenant member.
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := users.Create(t.Context(), tenantID, "member@x.com", hash, "", "member", false); err != nil {
		t.Fatalf("Create member: %v", err)
	}

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(sm, users, tenants)
	scH := pricelist.NewHandler(
		pricelist.NewService(conn),
		pricelist.NewImportService(conn, hub),
	)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Get("/price-list/versions", scH.ListVersions)
			pr.With(httpx.RequireRole("owner", "admin")).Post("/price-list/import/inspect", scH.Inspect)
			pr.With(httpx.RequireRole("owner", "admin")).Post("/price-list/import/commit", scH.Commit)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

const priceCSV = "Product,SKU,Price\nWidget,W1,9.99\nGadget,G1,4.50\n"

// postMultipart posts a multipart form with the CSV file plus the given extra
// text fields.
func postMultipart(t *testing.T, c *http.Client, url string, csv []byte, fields map[string]string) *http.Response {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if csv != nil {
		fw, err := w.CreateFormFile("file", "items.csv")
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write(csv); err != nil {
			t.Fatalf("write file part: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req, err := http.NewRequest("POST", url, &body)
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

func TestPriceListInspectOwnerReturnsHeaders(t *testing.T) {
	srv, uuid := newPriceListImportServer(t)
	c := loggedInClient(t, srv.URL) // o@x.com is the tenant owner
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/price-list/import/inspect", []byte(priceCSV), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inspect: want 200 got %d", resp.StatusCode)
	}
	var out struct {
		Headers    []string            `json:"headers"`
		SampleRows []map[string]string `json:"sampleRows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Headers) != 3 || out.Headers[0] != "Product" {
		t.Fatalf("headers = %v, want [Product SKU Price]", out.Headers)
	}
	if len(out.SampleRows) != 2 {
		t.Fatalf("sampleRows = %d, want 2", len(out.SampleRows))
	}

	// Inspect persists nothing: versions list stays empty.
	vr, err := c.Get(srv.URL + "/api/t/" + uuid + "/price-list/versions")
	if err != nil {
		t.Fatalf("GET versions: %v", err)
	}
	defer func() { _ = vr.Body.Close() }()
	var versions []map[string]any
	if err := json.NewDecoder(vr.Body).Decode(&versions); err != nil {
		t.Fatalf("decode versions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("inspect persisted %d versions, want 0", len(versions))
	}
}

func TestPriceListCommitOwnerCreatesVersion(t *testing.T) {
	srv, uuid := newPriceListImportServer(t)
	c := loggedInClient(t, srv.URL)
	mapping := `{"Product":"name","SKU":"code","Price":"unitPrice"}`
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/price-list/import/commit", []byte(priceCSV), map[string]string{
		"label": "Q1 catalogue", "mapping": mapping,
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("commit: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		VersionID string `json:"versionId"`
		ItemCount int    `json:"itemCount"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.VersionID == "" || out.ItemCount != 2 {
		t.Fatalf("summary = %+v, want non-empty versionId itemCount=2", out)
	}

	// The created version is queryable.
	vr, err := c.Get(srv.URL + "/api/t/" + uuid + "/price-list/versions")
	if err != nil {
		t.Fatalf("GET versions: %v", err)
	}
	defer func() { _ = vr.Body.Close() }()
	var versions []map[string]any
	if err := json.NewDecoder(vr.Body).Decode(&versions); err != nil {
		t.Fatalf("decode versions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(versions))
	}
}

func TestPriceListImportNonAdminForbidden(t *testing.T) {
	srv, uuid := newPriceListImportServer(t)
	c := jarClient(t)
	lr := login(t, c, srv.URL, "member@x.com", "password1")
	_ = lr.Body.Close()
	if lr.StatusCode != http.StatusOK {
		t.Fatalf("member login: want 200 got %d", lr.StatusCode)
	}
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/price-list/import/inspect", []byte(priceCSV), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin inspect: want 403 got %d", resp.StatusCode)
	}
}

func TestPriceListImportUnauthenticated401(t *testing.T) {
	srv, uuid := newPriceListImportServer(t)
	c := jarClient(t)
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/price-list/import/commit", []byte(priceCSV), map[string]string{
		"label": "x", "mapping": `{"Product":"name"}`,
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon commit: want 401 got %d", resp.StatusCode)
	}
}

func TestPriceListCommitMissingLabelRejected(t *testing.T) {
	srv, uuid := newPriceListImportServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/price-list/import/commit", []byte(priceCSV), map[string]string{
		"mapping": `{"Product":"name"}`,
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing label: want 400 got %d", resp.StatusCode)
	}
}
