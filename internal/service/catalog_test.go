package service

import (
	"context"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/xuri/excelize/v2"
)

func newCustomItemSvc(t *testing.T) (*CustomItemService, *realtime.Hub, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	hub := realtime.NewHub()
	return NewCustomItemService(conn, hub), hub, tenantID
}

func TestCustomItemCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newCustomItemSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	item, err := svc.Create(tctx(tenantID), repository.CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if item == nil {
		t.Fatal("Create returned nil item")
	}

	select {
	case e := <-ch:
		if e.Entity != "custom_item" || e.ID != item.ID || e.Action != "create" {
			t.Fatalf("event=%+v want custom_item/%d/create", e, item.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestCustomItemCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newCustomItemSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), repository.CustomItemInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

func TestCustomItemBulkDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID := newCustomItemSvc(t)
	ctx := tctx(tenantID)

	item, err := svc.Create(ctx, repository.CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe()
	defer unsub()

	if err := svc.BulkDelete(ctx, []int64{item.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "custom_item" || e.ID != 0 || e.Action != "bulk_delete" {
			t.Fatalf("event=%+v want custom_item/0/bulk_delete", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after BulkDelete")
	}
}

// catalogXLSX builds a synthetic NDIS-Support-Catalogue-shaped XLSX in-memory.
// Columns mirror the canonical headers the fixed parser expects; the three zone
// price columns drive the per-zone price rows. rows are [code,name,unit,cat,reg,
// national,remote,veryremote].
func catalogXLSX(t *testing.T, headers []string, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Sheet1"
	hdr := make([]any, len(headers))
	for i := range headers {
		hdr[i] = headers[i]
	}
	if err := f.SetSheetRow(sheet, "A1", &hdr); err != nil {
		t.Fatalf("SetSheetRow header: %v", err)
	}
	for i := range rows {
		cells := make([]any, len(rows[i]))
		for j := range rows[i] {
			cells[j] = rows[i][j]
		}
		if err := f.SetSheetRow(sheet, "A"+itoaTest(i+2), &cells); err != nil {
			t.Fatalf("SetSheetRow data: %v", err)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}
	return buf.Bytes()
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 { // bounded by the magnitude of n
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

var catalogHeaders = []string{
	"Support Item Number", "Support Item Name", "Unit",
	"Support Category", "Registration Group Name",
	"National", "Remote", "Very Remote",
}

func TestCatalogIngestCreatesVersionItemsAndPrices(t *testing.T) {
	conn := newTestDB(t)
	hub := realtime.NewHub()
	ingest := NewCatalogIngestService(conn, hub)
	read := NewSupportCatalogService(conn)

	data := catalogXLSX(t, catalogHeaders, [][]string{
		{"01_011_0107_1_1", "Assistance With Self-Care", "Hour", "Core", "Daily Living", "$67.56", "$94.58", "$101.34"},
		{"15_056_0128_1_3", "Assessment Recommendation", "Hour", "CB", "Therapeutic Supports", "Quote", "", "Quote"},
	})

	ch, unsub := hub.Subscribe()
	defer unsub()

	summary, err := ingest.IngestXLSX(context.Background(), data, "2025-26 v1.1", "2025-07-01", "catalogue.xlsx")
	if err != nil {
		t.Fatalf("IngestXLSX: %v", err)
	}
	if summary.ItemCount != 2 {
		t.Fatalf("ItemCount = %d, want 2", summary.ItemCount)
	}
	// 3 zone prices for row 1; row 2 emits all 3 zone columns too (each present
	// in the sheet), so 6 price rows total.
	if summary.PriceCount != 6 {
		t.Fatalf("PriceCount = %d, want 6", summary.PriceCount)
	}

	select {
	case e := <-ch:
		if e.Entity != "catalog_version" || e.Action != "ingest" || e.ID != summary.VersionID {
			t.Fatalf("event=%+v want catalog_version/ingest/%d", e, summary.VersionID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after ingest")
	}

	ctx := tctx(seedTenant(t, conn))
	versions, err := read.ListVersions(ctx)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(versions))
	}

	items, err := read.ListSupportItems(ctx, summary.VersionID)
	if err != nil {
		t.Fatalf("ListSupportItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}

	// Resolve the quotable item: its national price cap must be NULL (quotable),
	// while the fixed-price item must carry a numeric national cap.
	fixed, err := read.repo.ResolveZonePrice(ctx, summary.VersionID, "01_011_0107_1_1", "national")
	if err != nil || fixed == nil {
		t.Fatalf("ResolveZonePrice fixed: %v %+v", err, fixed)
	}
	if fixed.PriceCap == nil || *fixed.PriceCap != 67.56 {
		t.Fatalf("fixed national cap = %v, want 67.56", fixed.PriceCap)
	}
	quote, err := read.repo.ResolveZonePrice(ctx, summary.VersionID, "15_056_0128_1_3", "national")
	if err != nil || quote == nil {
		t.Fatalf("ResolveZonePrice quote: %v %+v", err, quote)
	}
	if quote.PriceCap != nil {
		t.Fatalf("quotable national cap = %v, want nil", *quote.PriceCap)
	}
	// Blank cell (remote) is also quotable → nil.
	remote, err := read.repo.ResolveZonePrice(ctx, summary.VersionID, "15_056_0128_1_3", "remote")
	if err != nil || remote == nil {
		t.Fatalf("ResolveZonePrice blank: %v %+v", err, remote)
	}
	if remote.PriceCap != nil {
		t.Fatalf("blank remote cap = %v, want nil", *remote.PriceCap)
	}
}

// TestCatalogIngestMissingColumnRejectsWholeUpload asserts that an XLSX missing
// a required column is rejected and NO version row is created (tx rollback).
func TestCatalogIngestMissingColumnRejectsWholeUpload(t *testing.T) {
	conn := newTestDB(t)
	ingest := NewCatalogIngestService(conn, realtime.NewHub())
	read := NewSupportCatalogService(conn)

	// Drop the required "Support Item Name" column.
	badHeaders := []string{"Support Item Number", "Unit", "National"}
	data := catalogXLSX(t, badHeaders, [][]string{
		{"01_011_0107_1_1", "Hour", "$67.56"},
	})

	if _, err := ingest.IngestXLSX(context.Background(), data, "bad", "2025-07-01", "bad.xlsx"); err == nil {
		t.Fatal("expected error for missing required column")
	}

	versions, err := read.ListVersions(tctx(seedTenant(t, conn)))
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("rollback failed: versions = %d, want 0", len(versions))
	}
}

// TestCatalogIngestNoDataRowsRejected asserts a header-only sheet is rejected.
func TestCatalogIngestNoDataRowsRejected(t *testing.T) {
	conn := newTestDB(t)
	ingest := NewCatalogIngestService(conn, realtime.NewHub())
	data := catalogXLSX(t, catalogHeaders, nil)
	if _, err := ingest.IngestXLSX(context.Background(), data, "empty", "2025-07-01", "empty.xlsx"); err == nil {
		t.Fatal("expected error for zero data rows")
	}
}

// SupportCatalogService is global, read-only reference data. With no catalogue
// ingested, ListVersions returns an empty (non-nil) slice.
func TestSupportCatalogListVersionsEmpty(t *testing.T) {
	conn := newTestDB(t)
	svc := NewSupportCatalogService(conn)

	versions, err := svc.ListVersions(tctx(seedTenant(t, conn)))
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("ListVersions = %d, want 0", len(versions))
	}
}
