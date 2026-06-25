package catalogue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/importer"
)

// maxSampleRows caps the Inspect preview so the payload stays small.
const maxSampleRows = 10

// InspectResult is the headers + a capped sample of data rows from an uploaded
// file. The SPA renders one mapping <select> per header. Inspect persists nothing.
type InspectResult struct {
	Headers    []string            `json:"headers"`
	SampleRows []map[string]string `json:"sampleRows"`
}

// ImportSummary is the JSON-friendly result of a catalogue import.
type ImportSummary struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
}

// Inspect parses an uploaded file and returns its headers plus a sample of up to
// maxSampleRows rows, WITHOUT writing anything. fileType is "csv" or "xlsx".
func (r *Repo) Inspect(data []byte, fileType, sheetName string, headerRow int) (*InspectResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("inspect: empty file")
	}
	headers, rows, err := importer.ParseRows(data, fileType, sheetName, headerRow)
	if err != nil {
		return nil, fmt.Errorf("inspect: %w", err)
	}
	sample := rows
	if len(sample) > maxSampleRows {
		sample = sample[:maxSampleRows]
	}
	return &InspectResult{Headers: headers, SampleRows: sample}, nil
}

// ImportMapped parses an uploaded file, applies the source-column->target-field
// mapping, and upserts each row into the catalogue BY CODE in one transaction.
// A known current code updates that item (copy-on-write rules apply); a new code
// (or no code) creates a fresh item. The whole upload is rejected (no partial
// state) when the required "name" target is unmapped or zero rows parse.
func (r *Repo) ImportMapped(ctx context.Context, tenantID string, data []byte, fileType, sheetName string, headerRow int, mapping map[string]string) (*ImportSummary, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("import: empty file")
	}
	headers, rows, err := importer.ParseRows(data, fileType, sheetName, headerRow)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	parsed, err := importer.ApplyMapping(headers, rows, mapping)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("import: no data rows")
	}

	summary := &ImportSummary{}
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for i := range parsed { // bounded by len(parsed)
			p := parsed[i]
			in := CatalogueItemInput{
				Code:      p.Code,
				Name:      p.Name,
				Unit:      p.Unit,
				Category:  p.Category,
				UnitPrice: p.UnitPrice,
				Taxable:   p.Taxable,
			}
			created, e := upsertByCode(ctx, q, tenantID, in)
			if e != nil {
				return fmt.Errorf("row %d (%q): %w", i, p.Name, e)
			}
			if created {
				summary.Created++
			} else {
				summary.Updated++
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "catalogue_item",
			EntityID:   "",
			Action:     "import",
			Changes:    audit.Changes(map[string]any{"created": summary.Created, "updated": summary.Updated}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	return summary, nil
}

// upsertByCode updates the current item for in.Code (copy-on-write), or creates a
// new item when the code is empty or unknown. Returns created=true on insert.
func upsertByCode(ctx context.Context, q *gen.Queries, tenantID string, in CatalogueItemInput) (bool, error) {
	if in.Code != "" {
		cur, err := q.GetCurrentCatalogueByCode(ctx, gen.GetCurrentCatalogueByCodeParams{TenantID: tenantID, Code: nz(in.Code)})
		if err == nil {
			if _, e := updateCoW(ctx, q, tenantID, cur.ID, in); e != nil {
				return false, e
			}
			return false, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("lookup by code: %w", err)
		}
	}
	if _, err := createVersion(ctx, q, tenantID, ids.New(), 1, in); err != nil {
		return false, err
	}
	return true, nil
}

func nz(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
