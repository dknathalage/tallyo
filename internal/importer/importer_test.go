package importer

import (
	"testing"

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
	_ = f.SetSheetRow(sheet, "A1", &[]any{"name", "sku", "rate"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"Widget", "W1", 10})
	_ = f.SetSheetRow(sheet, "A3", &[]any{"Gadget", "W2", 5})
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

func TestParseRowsEmpty(t *testing.T) {
	if _, _, err := ParseRows(nil, "csv", "", 1); err == nil {
		t.Fatal("want error for empty file")
	}
}

// TestParseRowsRagged covers the padding path: a data row with fewer cells than
// the header must have its missing trailing columns filled with "" rather than
// being absent from the map. J7's fixed parser depends on this guarantee.
func TestParseRowsRagged(t *testing.T) {
	data := []byte("name,sku,rate\nWidget,W1")
	headers, rows, err := ParseRows(data, "csv", "", 1)
	if err != nil {
		t.Fatalf("ParseRows: %v", err)
	}
	if len(headers) != 3 || len(rows) != 1 {
		t.Fatalf("headers=%v rows=%v", headers, rows)
	}
	if rows[0]["name"] != "Widget" || rows[0]["sku"] != "W1" {
		t.Fatalf("row 0 present cells: %v", rows[0])
	}
	v, ok := rows[0]["rate"]
	if !ok {
		t.Fatal("missing column should be present in the map")
	}
	if v != "" {
		t.Fatalf("missing column should pad to \"\", got %q", v)
	}
}

func TestParseFloat(t *testing.T) {
	cases := map[string]float64{
		"":           0,
		"abc":        0,
		"10":         10,
		"$1,234.50":  1234.50,
		" 5.5 ":      5.5,
		"-12.50":     -12.50,
		"-$1,200.00": -1200.00,
	}
	for in, want := range cases {
		if got := ParseFloat(in); got != want {
			t.Fatalf("ParseFloat(%q) = %v want %v", in, got, want)
		}
	}
}
