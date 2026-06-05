package importer

import "testing"

func sampleRows(headers []string, rows ...[]string) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		m := map[string]string{}
		for i, h := range headers {
			if i < len(r) {
				m[h] = r[i]
			}
		}
		out = append(out, m)
	}
	return out
}

func TestDetectMappingNDISStyle(t *testing.T) {
	headers := []string{"Support Item Number", "Support Item Name", "Support Category Name", "Unit", "ACT", "Remote", "Very Remote"}
	sample := sampleRows(headers,
		[]string{"01_011_0107_1_1", "Assistance With Self-Care", "Daily Activities", "H", "$67.56", "$94.58", "$101.34"},
	)
	s := DetectMapping(headers, sample)
	if s.Fields["Support Item Number"] != "sku" {
		t.Errorf("sku: got %q", s.Fields["Support Item Number"])
	}
	if s.Fields["Support Item Name"] != "name" {
		t.Errorf("name: got %q", s.Fields["Support Item Name"])
	}
	if s.Fields["Support Category Name"] != "category" {
		t.Errorf("category: got %q", s.Fields["Support Category Name"])
	}
	if s.Fields["Unit"] != "unit" {
		t.Errorf("unit: got %q", s.Fields["Unit"])
	}
	if s.BaseHeader != "ACT" {
		t.Errorf("base: got %q want ACT (leftmost price col)", s.BaseHeader)
	}
	gotTiers := map[string]bool{}
	for _, p := range s.PriceCols {
		gotTiers[p.Header] = true
		if p.SuggestName != p.Header {
			t.Errorf("suggest name: got %q want %q", p.SuggestName, p.Header)
		}
	}
	if !gotTiers["Remote"] || !gotTiers["Very Remote"] || gotTiers["ACT"] {
		t.Errorf("tiers: got %v", gotTiers)
	}
}

func TestDetectMappingGenericPriceAndTier(t *testing.T) {
	headers := []string{"name", "sku", "unit", "price", "premium"}
	sample := sampleRows(headers, []string{"Widget", "W1", "ea", "10.00", "12.50"})
	s := DetectMapping(headers, sample)
	if s.BaseHeader != "price" {
		t.Errorf("base: got %q want price", s.BaseHeader)
	}
	if len(s.PriceCols) != 1 || s.PriceCols[0].Header != "premium" {
		t.Errorf("tiers: got %+v want [premium]", s.PriceCols)
	}
}

func TestDetectMappingCodeColumnIsNotTier(t *testing.T) {
	headers := []string{"name", "category number", "price"}
	sample := sampleRows(headers,
		[]string{"A", "101", "5.00"},
		[]string{"B", "102", "6.00"},
	)
	s := DetectMapping(headers, sample)
	if len(s.PriceCols) != 0 {
		t.Errorf("integer code column must not be a tier: got %+v", s.PriceCols)
	}
	if s.BaseHeader != "price" {
		t.Errorf("base: got %q", s.BaseHeader)
	}
}

func TestDetectMappingMissingName(t *testing.T) {
	headers := []string{"sku", "price"}
	s := DetectMapping(headers, sampleRows(headers, []string{"X1", "9.99"}))
	for _, f := range s.Fields {
		if f == "name" {
			t.Fatalf("no name column should be detected")
		}
	}
	if s.BaseHeader != "price" {
		t.Errorf("base: got %q", s.BaseHeader)
	}
}
