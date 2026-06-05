package importer

import (
	"bytes"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/xuri/excelize/v2"
)

func TestParseRowsCSV(t *testing.T) {
	data := []byte("name,sku,rate\nWidget,W1,10\nGadget,W2,5")
	headers, rows, err := ParseRows(data, "csv", "", 1)
	if err != nil {
		t.Fatalf("ParseRows: %v", err)
	}
	if len(headers) != 3 || headers[0] != "name" || headers[2] != "rate" {
		t.Fatalf("headers: %v", headers)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows got %d", len(rows))
	}
	if rows[0]["name"] != "Widget" || rows[0]["sku"] != "W1" || rows[0]["rate"] != "10" {
		t.Fatalf("row 0: %v", rows[0])
	}
	if rows[1]["name"] != "Gadget" || rows[1]["rate"] != "5" {
		t.Fatalf("row 1: %v", rows[1])
	}
}

func TestParseRowsCSVHeaderRow2(t *testing.T) {
	data := []byte("junk title\nname,sku,rate\nWidget,W1,10")
	headers, rows, err := ParseRows(data, "", "", 2)
	if err != nil {
		t.Fatalf("ParseRows: %v", err)
	}
	if len(headers) != 3 || headers[0] != "name" {
		t.Fatalf("headers: %v", headers)
	}
	if len(rows) != 1 || rows[0]["name"] != "Widget" {
		t.Fatalf("rows: %v", rows)
	}
}

func TestParseRowsXLSX(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Sheet1"
	_ = f.SetSheetRow(sheet, "A1", &[]interface{}{"name", "sku", "rate"})
	_ = f.SetSheetRow(sheet, "A2", &[]interface{}{"Widget", "W1", 10})
	_ = f.SetSheetRow(sheet, "A3", &[]interface{}{"Gadget", "W2", 5})
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}
	headers, rows, err := ParseRows(buf.Bytes(), "xlsx", "", 1)
	if err != nil {
		t.Fatalf("ParseRows xlsx: %v", err)
	}
	if len(headers) != 3 || headers[0] != "name" {
		t.Fatalf("headers: %v", headers)
	}
	if len(rows) != 2 || rows[0]["name"] != "Widget" || rows[1]["sku"] != "W2" {
		t.Fatalf("rows: %v", rows)
	}
}

func TestApplyMapping(t *testing.T) {
	rows := []map[string]string{
		{"name": "Widget", "sku": "W1", "rate": "10"},
		{"name": "Gadget", "sku": "W2", "rate": "5"},
	}
	m := Mapping{
		Fields: map[string]string{"name": "name", "sku": "sku", "rate": "rate"},
	}
	mapped, errs, err := ApplyMapping(rows, m)
	if err != nil {
		t.Fatalf("ApplyMapping: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("want no errors got %v", errs)
	}
	if len(mapped) != 2 {
		t.Fatalf("want 2 mapped got %d", len(mapped))
	}
	if mapped[0].Name != "Widget" || mapped[0].Sku != "W1" || mapped[0].Rate != 10 {
		t.Fatalf("mapped[0]: %+v", mapped[0])
	}
}

func TestApplyMappingMissingName(t *testing.T) {
	rows := []map[string]string{
		{"name": "", "sku": "W1", "rate": "10"},
		{"name": "Gadget", "sku": "W2", "rate": "5"},
	}
	m := Mapping{
		Fields: map[string]string{"name": "name", "sku": "sku", "rate": "rate"},
	}
	mapped, errs, err := ApplyMapping(rows, m)
	if err != nil {
		t.Fatalf("ApplyMapping: %v", err)
	}
	if len(mapped) != 1 || mapped[0].Name != "Gadget" {
		t.Fatalf("want 1 mapped Gadget got %+v", mapped)
	}
	if len(errs) != 1 || errs[0].Row != 1 {
		t.Fatalf("want 1 error on row 1 got %v", errs)
	}
}

func TestApplyMappingTiersAndMetadata(t *testing.T) {
	rows := []map[string]string{
		{"name": "Widget", "sku": "W1", "rate": "10", "Gold Price": "20"},
	}
	m := Mapping{
		Fields:   map[string]string{"name": "name", "sku": "sku", "rate": "rate"},
		TierCols: map[string]string{"Gold Price": "Gold"},
	}
	mapped, errs, err := ApplyMapping(rows, m)
	if err != nil {
		t.Fatalf("ApplyMapping: %v", err)
	}
	if len(errs) != 0 || len(mapped) != 1 {
		t.Fatalf("unexpected: errs=%v mapped=%v", errs, mapped)
	}
	if mapped[0].TierRates["Gold"] != 20 {
		t.Fatalf("tier rate by name: %v", mapped[0].TierRates)
	}
}

func newCatalogAndTiers(t *testing.T) (*repository.CatalogRepo, *repository.RateTiersRepo) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "importer.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return repository.NewCatalog(conn), repository.NewRateTiers(conn)
}

func TestDiff(t *testing.T) {
	cat, _ := newCatalogAndTiers(t)
	if _, err := cat.Create(t.Context(), repository.CatalogItemInput{Name: "Widget", Sku: "W1", Rate: 10}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mapped := []MappedRow{
		{Name: "Widget", Sku: "W1", Rate: 99},
		{Name: "New", Sku: "W3", Rate: 1},
	}
	res, err := Diff(t.Context(), cat, mapped, 0)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(res.New) != 1 || res.New[0].Sku != "W3" {
		t.Fatalf("new: %+v", res.New)
	}
	if len(res.Updated) != 1 || res.Updated[0].Existing.Sku != "W1" {
		t.Fatalf("updated: %+v", res.Updated)
	}
	if res.UnchangedCount != 0 {
		t.Fatalf("unchanged: %d", res.UnchangedCount)
	}
	if res.Summary.Total != 2 || res.Summary.New != 1 || res.Summary.Updated != 1 {
		t.Fatalf("summary: %+v", res.Summary)
	}
}

func TestDiffUnchanged(t *testing.T) {
	cat, _ := newCatalogAndTiers(t)
	if _, err := cat.Create(t.Context(), repository.CatalogItemInput{Name: "Widget", Sku: "W1", Rate: 10}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mapped := []MappedRow{
		{Name: "Widget", Sku: "w1", Rate: 10},
	}
	res, err := Diff(t.Context(), cat, mapped, 2)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if res.UnchangedCount != 1 || len(res.New) != 0 || len(res.Updated) != 0 {
		t.Fatalf("res: %+v", res)
	}
	if res.Summary.Unchanged != 1 || res.Summary.Errors != 2 {
		t.Fatalf("summary: %+v", res.Summary)
	}
}

func TestCommit(t *testing.T) {
	cat, tiers := newCatalogAndTiers(t)
	existing, err := cat.Create(t.Context(), repository.CatalogItemInput{Name: "Widget", Sku: "W1", Rate: 10})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	diff := DiffResult{
		New: []MappedRow{
			{Name: "New", Sku: "W3", Rate: 1, Metadata: map[string]string{"color": "blue"}, TierRates: map[string]float64{}},
		},
		Updated: []UpdatedItem{
			{Existing: existing, Incoming: MappedRow{Name: "Widget", Sku: "W1", Rate: 99}},
		},
	}
	res, err := Commit(t.Context(), cat, tiers, diff, true)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Inserted != 1 || res.Updated != 1 {
		t.Fatalf("res: %+v", res)
	}
	if res.BatchID == "" {
		t.Fatal("empty batchID")
	}
	items, err := cat.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items got %d", len(items))
	}
}

func TestCommitSkipUpdatesWhenDisabled(t *testing.T) {
	cat, tiers := newCatalogAndTiers(t)
	existing, err := cat.Create(t.Context(), repository.CatalogItemInput{Name: "Widget", Sku: "W1", Rate: 10})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	diff := DiffResult{
		Updated: []UpdatedItem{
			{Existing: existing, Incoming: MappedRow{Name: "Widget", Sku: "W1", Rate: 99}},
		},
	}
	res, err := Commit(t.Context(), cat, tiers, diff, false)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Updated != 0 {
		t.Fatalf("want 0 updated got %d", res.Updated)
	}
	item, err := cat.Get(t.Context(), existing.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.Rate != 10 {
		t.Fatalf("rate should be unchanged, got %v", item.Rate)
	}
}

func TestCommitCreatesTierByName(t *testing.T) {
	cat, tiers := newCatalogAndTiers(t)
	diff := DiffResult{New: []MappedRow{{
		Name: "Item", Sku: "S1", Rate: 10,
		TierRates: map[string]float64{"Remote": 15},
	}}}
	res, err := Commit(t.Context(), cat, tiers, diff, false)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Inserted != 1 {
		t.Fatalf("inserted: %d", res.Inserted)
	}
	all, _ := tiers.List(t.Context())
	found := false
	for _, tr := range all {
		if tr.Name == "Remote" {
			found = true
		}
	}
	if !found {
		t.Errorf("tier 'Remote' should have been created")
	}
}

func TestMetadataJSON(t *testing.T) {
	if got := metadataJSON(nil); got != "{}" {
		t.Fatalf("nil: %q", got)
	}
	if got := metadataJSON(map[string]string{}); got != "{}" {
		t.Fatalf("empty: %q", got)
	}
	got := metadataJSON(map[string]string{"color": "red"})
	if !bytes.Contains([]byte(got), []byte(`"color":"red"`)) {
		t.Fatalf("got %q", got)
	}
}
