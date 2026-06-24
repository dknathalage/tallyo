package importer

import (
	"fmt"
	"strings"
)

// ImportRow is one generic catalogue item parsed from an uploaded file.
type ImportRow struct {
	Name      string
	Code      string
	Unit      string
	Category  string
	UnitPrice float64
	Taxable   bool
}

// TargetFields are the catalogue fields a source column can map to. "name" is
// the only required target. Exported so callers (e.g. the map-import Smart) can
// validate a proposed mapping against the same set ApplyMapping enforces.
func TargetFields() []string {
	return []string{"name", "code", "unit", "category", "unitPrice", "taxable"}
}

var validTargets = func() map[string]bool {
	m := make(map[string]bool, 6)
	for _, t := range TargetFields() {
		m[t] = true
	}
	return m
}()

// ApplyMapping turns parsed rows into ImportRows using a sourceHeader→targetField
// map. "name" is required; unmapped/empty cells are zero values. taxable defaults
// to true (generic items are taxable unless the source says otherwise).
// Each item carries a single price column (unitPrice).
func ApplyMapping(headers []string, rows []map[string]string, mapping map[string]string) ([]ImportRow, error) {
	if len(mapping) == 0 {
		return nil, fmt.Errorf("import mapping is empty")
	}
	hasName := false
	for _, target := range mapping {
		if !validTargets[target] {
			return nil, fmt.Errorf("unknown target field %q", target)
		}
		if target == "name" {
			hasName = true
		}
	}
	if !hasName {
		return nil, fmt.Errorf("a source column must map to the required field \"name\"")
	}
	out := make([]ImportRow, 0, len(rows))
	for i := range rows {
		r := ImportRow{Taxable: true}
		for header, target := range mapping {
			cell := strings.TrimSpace(rows[i][header])
			switch target {
			case "name":
				r.Name = cell
			case "code":
				r.Code = cell
			case "unit":
				r.Unit = cell
			case "category":
				r.Category = cell
			case "unitPrice":
				r.UnitPrice = ParseFloat(cell)
			case "taxable":
				r.Taxable = !(cell == "" || cell == "0" || strings.EqualFold(cell, "false") || strings.EqualFold(cell, "no"))
			}
		}
		if r.Name == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no data rows with a name were found")
	}
	return out, nil
}
