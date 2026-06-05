package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/importer"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/go-chi/chi/v5"
)

// newImportServer wires the import routes behind RequireAuth, seeds one catalog
// item (SKU W1, rate 10) and a column mapping (name/sku/rate), returning the
// server and the mapping id.
func newImportServer(t *testing.T) (*httptest.Server, int64) {
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
	if _, err := catalogRepo.Create(t.Context(), repository.CatalogItemInput{Name: "Widget", Sku: "W1", Rate: 10}); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	mappings := repository.NewColumnMappings(conn)
	m, err := mappings.Create(t.Context(), repository.ColumnMappingInput{
		Name:       "test",
		EntityType: "catalog",
		Mapping:    `{"name":"name","sku":"sku","rate":"rate"}`,
		FileType:   "csv",
		HeaderRow:  1,
	})
	if err != nil {
		t.Fatalf("seed mapping: %v", err)
	}

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	impH := NewImportHandler(catalogRepo, mappings)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Post("/import/catalog/preview", impH.Preview)
			pr.Post("/import/catalog/commit", impH.Commit)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, m.ID
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
	for k, v := range fields {
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

func TestImportPreview(t *testing.T) {
	srv, mappingID := newImportServer(t)
	c := loggedInClient(t, srv.URL)
	csvBody := "name,sku,rate\nWidget,W1,99\nGadget,W3,5"
	body, ct := buildImportForm(t, csvBody, map[string]string{"mappingId": fmt.Sprintf("%d", mappingID)})
	resp := postMultipart(t, c, srv.URL+"/api/import/catalog/preview", body, ct)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", resp.StatusCode)
	}
	var diff importer.DiffResult
	if err := json.NewDecoder(resp.Body).Decode(&diff); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(diff.New) != 1 || diff.New[0].Sku != "W3" {
		t.Fatalf("new: %+v", diff.New)
	}
	if len(diff.Updated) != 1 || diff.Updated[0].Existing.Sku != "W1" {
		t.Fatalf("updated: %+v", diff.Updated)
	}
	if diff.Summary.Total != 2 {
		t.Fatalf("summary: %+v", diff.Summary)
	}
}

func TestImportCommit(t *testing.T) {
	srv, mappingID := newImportServer(t)
	c := loggedInClient(t, srv.URL)
	csvBody := "name,sku,rate\nWidget,W1,99\nGadget,W3,5"
	body, ct := buildImportForm(t, csvBody, map[string]string{
		"mappingId":      fmt.Sprintf("%d", mappingID),
		"updateExisting": "true",
	})
	resp := postMultipart(t, c, srv.URL+"/api/import/catalog/commit", body, ct)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", resp.StatusCode)
	}
	var res importer.CommitResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Inserted != 1 || res.Updated != 1 {
		t.Fatalf("res: %+v", res)
	}
	if res.BatchID == "" {
		t.Fatal("empty batchId")
	}
}

func TestImportPreviewNoFile(t *testing.T) {
	srv, mappingID := newImportServer(t)
	c := loggedInClient(t, srv.URL)
	body, ct := buildImportForm(t, "", map[string]string{"mappingId": fmt.Sprintf("%d", mappingID)})
	resp := postMultipart(t, c, srv.URL+"/api/import/catalog/preview", body, ct)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", resp.StatusCode)
	}
}

func TestImportPreviewBadMapping(t *testing.T) {
	srv, _ := newImportServer(t)
	c := loggedInClient(t, srv.URL)
	body, ct := buildImportForm(t, "name,sku,rate\nWidget,W1,99", map[string]string{"mappingId": "99999"})
	resp := postMultipart(t, c, srv.URL+"/api/import/catalog/preview", body, ct)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", resp.StatusCode)
	}
}

func TestImportPreviewUnauthorized(t *testing.T) {
	srv, mappingID := newImportServer(t)
	c := jarClient(t)
	body, ct := buildImportForm(t, "name,sku,rate\nWidget,W1,99", map[string]string{"mappingId": fmt.Sprintf("%d", mappingID)})
	resp := postMultipart(t, c, srv.URL+"/api/import/catalog/preview", body, ct)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", resp.StatusCode)
	}
}
