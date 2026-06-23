package pricelist

import (
	"context"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
)

// csvBytes builds a small CSV file in memory.
func csvBytes(lines ...string) []byte {
	out := ""
	for i := range lines {
		out += lines[i] + "\n"
	}
	return []byte(out)
}

func TestImportInspectPersistsNothing(t *testing.T) {
	conn := newTestDB(t)
	svc := NewImportService(conn, realtime.NewHub())
	read := NewService(conn)

	data := csvBytes("Product,SKU,Price", "Widget,W1,9.99", "Gadget,G1,4.50")
	res, err := svc.Inspect(data, "csv", "", 1)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(res.Headers) != 3 || res.Headers[0] != "Product" {
		t.Fatalf("headers = %v, want [Product SKU Price]", res.Headers)
	}
	if len(res.SampleRows) != 2 {
		t.Fatalf("sampleRows = %d, want 2", len(res.SampleRows))
	}

	versions, err := read.ListVersions(tctx(seedTenant(t, conn)))
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("Inspect persisted %d versions, want 0", len(versions))
	}
}

func TestImportInspectCapsSample(t *testing.T) {
	conn := newTestDB(t)
	svc := NewImportService(conn, realtime.NewHub())
	lines := []string{"Name,Price"}
	for i := 0; i < 25; i++ {
		lines = append(lines, "Item,1.00")
	}
	res, err := svc.Inspect(csvBytes(lines...), "csv", "", 1)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(res.SampleRows) > 10 {
		t.Fatalf("sampleRows = %d, want ≤ 10", len(res.SampleRows))
	}
}

func TestImportMappedCreatesVersionAndItems(t *testing.T) {
	conn := newTestDB(t)
	hub := realtime.NewHub()
	svc := NewImportService(conn, hub)
	read := NewService(conn)

	ch, unsub := hub.Subscribe(1)
	defer unsub()

	data := csvBytes("Product,SKU,Unit,Cat,Price", "Widget,W1,Each,Hardware,9.99", "Gadget,G1,Each,Hardware,4.50")
	mapping := map[string]string{"Product": "name", "SKU": "code", "Unit": "unit", "Cat": "category", "Price": "unitPrice"}

	summary, err := svc.ImportMapped(context.Background(), data, "csv", "", 1, mapping, "Q1 catalogue")
	if err != nil {
		t.Fatalf("ImportMapped: %v", err)
	}
	if summary.ItemCount != 2 {
		t.Fatalf("ItemCount = %d, want 2", summary.ItemCount)
	}

	select {
	case e := <-ch:
		if e.Entity != "price_list_version" || e.UUID != summary.VersionUUID {
			t.Fatalf("event=%+v want price_list_version/%s", e, summary.VersionUUID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after import")
	}

	ctx := tctx(seedTenant(t, conn))
	versions, err := read.ListVersions(ctx)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(versions))
	}
	items, err := read.ListItemsByVersionUUID(ctx, summary.VersionUUID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	// unit_price + category persisted; taxable defaults true.
	var widget *Item
	for i := range items {
		if items[i].Code == "W1" {
			widget = items[i]
		}
	}
	if widget == nil {
		t.Fatal("W1 not found")
	}
	if widget.UnitPrice == nil || *widget.UnitPrice != 9.99 {
		t.Fatalf("W1 unitPrice = %v, want 9.99", widget.UnitPrice)
	}
	if widget.Category != "Hardware" {
		t.Fatalf("W1 category = %q, want Hardware", widget.Category)
	}
	if !widget.Taxable {
		t.Fatalf("W1 taxable = false, want true")
	}
	// No item_prices written by generic import.
	prices, err := read.ListPricesByItemUUID(ctx, widget.UUID)
	if err != nil {
		t.Fatalf("ListPrices: %v", err)
	}
	if len(prices) != 0 {
		t.Fatalf("prices = %d, want 0 (generic import writes no zone prices)", len(prices))
	}
}

func TestImportMappedMissingNameRejected(t *testing.T) {
	conn := newTestDB(t)
	svc := NewImportService(conn, realtime.NewHub())
	read := NewService(conn)

	data := csvBytes("SKU,Price", "W1,9.99")
	mapping := map[string]string{"SKU": "code", "Price": "unitPrice"} // no name
	if _, err := svc.ImportMapped(context.Background(), data, "csv", "", 1, mapping, "bad"); err == nil {
		t.Fatal("expected error when no column maps to name")
	}
	versions, err := read.ListVersions(tctx(seedTenant(t, conn)))
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("rollback failed: versions = %d, want 0", len(versions))
	}
}

func TestImportMappedEmptyRowsRollback(t *testing.T) {
	conn := newTestDB(t)
	svc := NewImportService(conn, realtime.NewHub())
	read := NewService(conn)

	data := csvBytes("Product,Price", ",1.00", "  ,2.00") // all rows blank name
	mapping := map[string]string{"Product": "name", "Price": "unitPrice"}
	if _, err := svc.ImportMapped(context.Background(), data, "csv", "", 1, mapping, "empty"); err == nil {
		t.Fatal("expected error when no named rows parse")
	}
	versions, err := read.ListVersions(tctx(seedTenant(t, conn)))
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("rollback failed: versions = %d, want 0", len(versions))
	}
}
