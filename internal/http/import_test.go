package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/importer"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newImportServer wires the import routes (parse/preview/commit) behind
// RequireAuth alongside read routes for catalog and rate-tiers so commit tests
// can verify their effects.
func newImportServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "import.db"))
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

	catalogRepo := repository.NewCatalog(conn)
	rateTiersRepo := repository.NewRateTiers(conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	impH := NewImportHandler(catalogRepo, rateTiersRepo)
	catH := NewCatalogHandler(service.NewCatalogService(conn, hub))
	rtH := NewRateTierHandler(service.NewRateTierService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Post("/catalog/import/parse", impH.Parse)
			pr.Post("/catalog/import/preview", impH.Preview)
			pr.Post("/catalog/import/commit", impH.Commit)
			pr.Get("/catalog", catH.List)
			pr.Get("/rate-tiers", rtH.List)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// buildImportForm builds a multipart body with an optional CSV file plus the
// given form fields.
func buildImportForm(t *testing.T, csvBody string, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	if csvBody != "" {
		fw, err := mw.CreateFormFile("file", "items.csv")
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write([]byte(csvBody)); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	for k, v := range fields { // bounded by len(fields)
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return body, mw.FormDataContentType()
}

func postMultipart(t *testing.T, c *http.Client, url string, body *bytes.Buffer, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func TestImportParseSuggests(t *testing.T) {
	srv := newImportServer(t)
	c := loggedInClient(t, srv.URL)
	body, ct := buildImportForm(t, "name,sku,price\nWidget,W1,10.00", nil)
	resp := postMultipart(t, c, srv.URL+"/api/catalog/import/parse", body, ct)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", resp.StatusCode)
	}
	var out struct {
		Suggestion importer.Suggestion `json:"suggestion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := out.Suggestion.Fields["price"]; got != "rate" {
		t.Fatalf("suggestion.fields[price]: want rate got %q (%+v)", got, out.Suggestion.Fields)
	}
}

func TestImportPreviewMissingName(t *testing.T) {
	srv := newImportServer(t)
	c := loggedInClient(t, srv.URL)
	// File has no name column and the mapping omits a name field, so every row
	// is a row error.
	mapping := `{"fields":{"sku":"sku","rate":"price"},"fileType":"csv","headerRow":1}`
	body, ct := buildImportForm(t, "sku,price\nW1,10.00\nW2,20.00", map[string]string{"mapping": mapping})
	resp := postMultipart(t, c, srv.URL+"/api/catalog/import/preview", body, ct)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", resp.StatusCode)
	}
	var diff importer.DiffResult
	if err := json.NewDecoder(resp.Body).Decode(&diff); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if diff.Summary.Errors == 0 {
		t.Fatalf("want summary.errors > 0 got %+v", diff.Summary)
	}
}

func TestImportCommitCreatesItemAndTier(t *testing.T) {
	srv := newImportServer(t)
	c := loggedInClient(t, srv.URL)
	mapping := `{"fields":{"name":"name","sku":"sku","rate":"price"},"tierCols":{"Remote":"Remote"},"fileType":"csv","headerRow":1}`
	csvBody := "name,sku,price,Remote\nWidget,W1,10.00,12.00"
	body, ct := buildImportForm(t, csvBody, map[string]string{"mapping": mapping})
	resp := postMultipart(t, c, srv.URL+"/api/catalog/import/commit", body, ct)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", resp.StatusCode)
	}
	var res importer.CommitResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode commit: %v", err)
	}
	if res.Inserted != 1 {
		t.Fatalf("inserted: want 1 got %+v", res)
	}

	// The item should now appear in the catalog.
	cr := get(t, c, srv.URL+"/api/catalog")
	defer func() { _ = cr.Body.Close() }()
	var items []struct {
		Name string `json:"name"`
		Sku  string `json:"sku"`
	}
	if err := json.NewDecoder(cr.Body).Decode(&items); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(items) != 1 || items[0].Sku != "W1" {
		t.Fatalf("catalog: want [W1] got %+v", items)
	}

	// The referenced tier should have been created.
	tr := get(t, c, srv.URL+"/api/rate-tiers")
	defer func() { _ = tr.Body.Close() }()
	var tiers []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(tr.Body).Decode(&tiers); err != nil {
		t.Fatalf("decode tiers: %v", err)
	}
	found := false
	for _, ti := range tiers { // bounded by len(tiers)
		if ti.Name == "Remote" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rate-tiers: want Remote got %+v", tiers)
	}
}

func TestImportPreviewUnauthorized(t *testing.T) {
	srv := newImportServer(t)
	c := jarClient(t)
	mapping := `{"fields":{"name":"name"},"fileType":"csv","headerRow":1}`
	body, ct := buildImportForm(t, "name\nWidget", map[string]string{"mapping": mapping})
	resp := postMultipart(t, c, srv.URL+"/api/catalog/import/preview", body, ct)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", resp.StatusCode)
	}
}
