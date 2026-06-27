package app

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/catalogue"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// newCatalogueImportServer wires the per-tenant catalogue routes (CRUD + the
// owner/admin upload-and-map import). seedTenantOwner creates the "o@x.com"
// owner; we additionally seed a non-owner/admin "member" so the 403 path is
// exercised.
func newCatalogueImportServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "catalogue_import.db")
	users, tenantID, _, tenantUUID := seedTenantOwner(t, conn)

	if _, err := users.Create(t.Context(), tenantID, "member@x.com", "uid-member", "", "member", false); err != nil {
		t.Fatalf("Create member: %v", err)
	}

	v := newStubVerifier()
	tenants := auth.NewTenants(conn)
	catH := catalogue.NewHandler(catalogue.NewService(conn))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants))
			catH.Routes(pr)
		})
	})

	srv := httptest.NewServer(router)
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

func catalogueCount(t *testing.T, c *http.Client, base, uuid string) int {
	t.Helper()
	vr, err := c.Get(base + "/api/t/" + uuid + "/catalogue")
	if err != nil {
		t.Fatalf("GET catalogue: %v", err)
	}
	defer func() { _ = vr.Body.Close() }()
	var items []map[string]any
	if err := json.NewDecoder(vr.Body).Decode(&items); err != nil {
		t.Fatalf("decode catalogue: %v", err)
	}
	return len(items)
}

func TestCatalogueImportInspectOwnerReturnsHeaders(t *testing.T) {
	srv, uuid := newCatalogueImportServer(t)
	c := loggedInClient(t, srv.URL) // o@x.com is the tenant owner
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/import/inspect", []byte(priceCSV), nil)
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
	// Inspect persists nothing: the catalogue stays empty.
	if n := catalogueCount(t, c, srv.URL, uuid); n != 0 {
		t.Fatalf("inspect persisted %d items, want 0", n)
	}
}

func TestCatalogueImportCommitOwnerCreatesItems(t *testing.T) {
	srv, uuid := newCatalogueImportServer(t)
	c := loggedInClient(t, srv.URL)
	mapping := `{"Product":"name","SKU":"code","Price":"unitPrice"}`
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/import/commit", []byte(priceCSV), map[string]string{
		"mapping": mapping,
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("commit: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		Created int `json:"created"`
		Updated int `json:"updated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Created != 2 || out.Updated != 0 {
		t.Fatalf("summary = %+v, want created=2 updated=0", out)
	}
	// The imported items are queryable in the catalogue.
	if n := catalogueCount(t, c, srv.URL, uuid); n != 2 {
		t.Fatalf("catalogue items = %d, want 2", n)
	}
}

func TestCatalogueImportNonAdminForbidden(t *testing.T) {
	srv, uuid := newCatalogueImportServer(t)
	c := bearerClient(memberToken)
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/import/inspect", []byte(priceCSV), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin inspect: want 403 got %d", resp.StatusCode)
	}
}

func TestCatalogueImportUnauthenticated401(t *testing.T) {
	srv, uuid := newCatalogueImportServer(t)
	c := jarClient(t)
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/import/commit", []byte(priceCSV), map[string]string{
		"mapping": `{"Product":"name"}`,
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon commit: want 401 got %d", resp.StatusCode)
	}
}

func TestCatalogueImportCommitMissingMappingRejected(t *testing.T) {
	srv, uuid := newCatalogueImportServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postMultipart(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/import/commit", []byte(priceCSV), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing mapping: want 400 got %d", resp.StatusCode)
	}
}
