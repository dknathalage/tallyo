package pricelist

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// newCatalogHandler builds a handler over a fresh DB and seeds one version with
// one priced item. Returns the handler, tenant id, and the seeded version + item
// UUIDs.
func newCatalogHandler(t *testing.T) (h *Handler, tenantID int64, versionUUID, itemUUID string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID = seedTenant(t, conn)
	q := gen.New(conn)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	vUUID := uuid.NewString()
	v, err := q.CreatePriceListVersion(ctx, gen.CreatePriceListVersionParams{
		Uuid: vUUID, Label: "2025-26", EffectiveFrom: "2025-07-01", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreatePriceListVersion: %v", err)
	}
	iUUID := uuid.NewString()
	si, err := q.CreateItem(ctx, gen.CreateItemParams{
		Uuid: iUUID, PriceListVersionID: v.ID, Code: "01_011_0107_1_1", Name: "Item", Taxable: 0,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if _, err := q.CreateItemPrice(ctx, gen.CreateItemPriceParams{
		ItemID: si.ID, Zone: "national", PriceCap: sql.NullFloat64{Float64: 100, Valid: true},
	}); err != nil {
		t.Fatalf("CreateItemPrice: %v", err)
	}
	svc := NewService(conn)
	return NewHandler(svc, nil), tenantID, vUUID, iUUID
}

// mountCatalog returns a router with the slice routes mounted and a middleware
// that attaches the tenant id to every request context (standing in for auth).
func mountCatalog(h *Handler, tenantID int64) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	h.Routes(r)
	return r
}

func TestCatalogListItemsByVersionUUID(t *testing.T) {
	h, tenantID, versionUUID, itemUUID := newCatalogHandler(t)
	srv := httptest.NewServer(mountCatalog(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/price-list/versions/" + versionUUID + "/items")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	var items []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items want 1", len(items))
	}
	if items[0]["id"] != itemUUID {
		t.Fatalf("item id=%v want uuid %q", items[0]["id"], itemUUID)
	}
	if items[0]["priceListVersionId"] != versionUUID {
		t.Fatalf("priceListVersionId=%v want version uuid %q", items[0]["priceListVersionId"], versionUUID)
	}
}

func TestCatalogListPricesByItemUUID(t *testing.T) {
	h, tenantID, _, itemUUID := newCatalogHandler(t)
	srv := httptest.NewServer(mountCatalog(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/price-list/items/" + itemUUID + "/prices")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	var prices []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&prices); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("got %d prices want 1", len(prices))
	}
	if _, ok := prices[0]["id"]; ok {
		t.Fatalf("price should not expose an id, got %v", prices[0]["id"])
	}
	if prices[0]["zone"] != "national" {
		t.Fatalf("zone=%v want national", prices[0]["zone"])
	}
}

func TestCatalogListItemsUnknownVersionUUID404(t *testing.T) {
	h, tenantID, _, _ := newCatalogHandler(t)
	srv := httptest.NewServer(mountCatalog(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/price-list/versions/3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c/items")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", res.StatusCode)
	}
}

func TestCatalogListPricesUnknownItemUUID404(t *testing.T) {
	h, tenantID, _, _ := newCatalogHandler(t)
	srv := httptest.NewServer(mountCatalog(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/price-list/items/3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c/prices")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", res.StatusCode)
	}
}

func TestCatalogListItemsNonUUID400(t *testing.T) {
	h, tenantID, _, _ := newCatalogHandler(t)
	srv := httptest.NewServer(mountCatalog(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/price-list/versions/123/items")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", res.StatusCode)
	}
}

func TestCatalogListPricesNonUUID400(t *testing.T) {
	h, tenantID, _, _ := newCatalogHandler(t)
	srv := httptest.NewServer(mountCatalog(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/price-list/items/123/prices")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", res.StatusCode)
	}
}
