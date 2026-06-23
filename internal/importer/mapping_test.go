package importer

import "testing"

func TestApplyMapping(t *testing.T) {
	headers := []string{"Product", "SKU", "Price"}
	rows := []map[string]string{
		{"Product": "Widget", "SKU": "W1", "Price": "9.99"},
		{"Product": "", "SKU": "W2", "Price": "1.00"}, // blank name → skipped
	}
	mapping := map[string]string{"Product": "name", "SKU": "code", "Price": "unitPrice"}

	got, err := ApplyMapping(headers, rows, mapping)
	if err != nil {
		t.Fatalf("ApplyMapping: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (blank-name row skipped)", len(got))
	}
	want := ImportRow{Name: "Widget", Code: "W1", UnitPrice: 9.99, Taxable: true}
	if got[0] != want {
		t.Fatalf("row = %+v, want %+v", got[0], want)
	}
}

func TestApplyMappingMissingNameRejected(t *testing.T) {
	headers := []string{"SKU", "Price"}
	rows := []map[string]string{{"SKU": "W1", "Price": "1"}}
	mapping := map[string]string{"SKU": "code", "Price": "unitPrice"}
	if _, err := ApplyMapping(headers, rows, mapping); err == nil {
		t.Fatal("expected error when no column maps to required \"name\"")
	}
}

func TestApplyMappingUnknownTargetRejected(t *testing.T) {
	headers := []string{"Product", "Bogus"}
	rows := []map[string]string{{"Product": "Widget", "Bogus": "x"}}
	mapping := map[string]string{"Product": "name", "Bogus": "nope"}
	if _, err := ApplyMapping(headers, rows, mapping); err == nil {
		t.Fatal("expected error for unknown target field")
	}
}

func TestApplyMappingEmptyMappingRejected(t *testing.T) {
	if _, err := ApplyMapping([]string{"A"}, []map[string]string{{"A": "x"}}, map[string]string{}); err == nil {
		t.Fatal("expected error for empty mapping")
	}
}

func TestApplyMappingNoNamedRowsRejected(t *testing.T) {
	headers := []string{"Product"}
	rows := []map[string]string{{"Product": ""}, {"Product": "   "}}
	mapping := map[string]string{"Product": "name"}
	if _, err := ApplyMapping(headers, rows, mapping); err == nil {
		t.Fatal("expected error when every row has a blank name")
	}
}

func TestApplyMappingTaxableAndCategory(t *testing.T) {
	headers := []string{"Name", "Cat", "Unit", "Tax"}
	rows := []map[string]string{
		{"Name": "A", "Cat": "Core", "Unit": "Hour", "Tax": "no"},
		{"Name": "B", "Cat": "CB", "Unit": "Each", "Tax": "yes"},
	}
	mapping := map[string]string{"Name": "name", "Cat": "category", "Unit": "unit", "Tax": "taxable"}
	got, err := ApplyMapping(headers, rows, mapping)
	if err != nil {
		t.Fatalf("ApplyMapping: %v", err)
	}
	if got[0].Taxable {
		t.Fatalf("row0 taxable=true, want false (\"no\")")
	}
	if !got[1].Taxable {
		t.Fatalf("row1 taxable=false, want true (\"yes\")")
	}
	if got[0].Category != "Core" || got[0].Unit != "Hour" {
		t.Fatalf("row0 = %+v, want Category=Core Unit=Hour", got[0])
	}
}
