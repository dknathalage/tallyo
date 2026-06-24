package smarts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dknathalage/tallyo/internal/importer"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

const mapImportSystem = `You map the columns of an uploaded price-list file to the catalogue's fields.

Target fields: name (required), code, unit, category, unitPrice, taxable.

Rules:
- Map each source header to the single best-fitting target field.
- Use the sample rows to judge what each column contains.
- Only emit a mapping for headers you are confident about; omit the rest.
- Never map two headers to the same target. When done, call map_columns.`

// MapInput is the request for the map-import Smart: the detected headers and a
// sample of rows from the uploaded file (from the existing inspect step).
type MapInput struct {
	Headers []string            `json:"headers"`
	Rows    []map[string]string `json:"rows"`
}

// MapResult is the proposed source-header → target-field mapping, ready to
// pre-fill the import wizard. Only known target fields survive.
type MapResult struct {
	Mappings map[string]string `json:"mappings"`
}

type mapColumnsCommit struct {
	Mappings []struct {
		Header string `json:"header"`
		Field  string `json:"field"`
	} `json:"mappings"`
}

// MapImport proposes a column→field mapping for the price-list import wizard.
// One forced-tool call; the result is validated against the importer's known
// target fields (unknowns dropped) — the user adjusts and commits via the
// existing import flow.
func (s *Service) MapImport(ctx context.Context, in MapInput) (MapResult, error) {
	_ = reqctx.MustTenant(ctx) // entry-point tenant guard
	if len(in.Headers) == 0 {
		return MapResult{}, fmt.Errorf("%w: no headers to map", ErrNoData)
	}

	out, err := s.llm.Propose(ctx, ProposeRequest{
		System: mapImportSystem,
		User:   buildMapUser(in),
		Force:  mapColumnsTool,
	})
	if err != nil {
		return MapResult{}, err
	}

	var commit mapColumnsCommit
	if err := json.Unmarshal(out, &commit); err != nil {
		return MapResult{}, fmt.Errorf("smarts: parse mapping: %w", err)
	}

	valid := make(map[string]bool, len(importer.TargetFields()))
	for _, f := range importer.TargetFields() {
		valid[f] = true
	}
	known := make(map[string]bool, len(in.Headers))
	for _, h := range in.Headers {
		known[h] = true
	}

	mappings := make(map[string]string)
	for _, m := range commit.Mappings { // bounded by len(commit.Mappings)
		if valid[m.Field] && known[m.Header] {
			mappings[m.Header] = m.Field
		}
	}
	return MapResult{Mappings: mappings}, nil
}

// buildMapUser renders the headers + a small sample for the model.
func buildMapUser(in MapInput) string {
	var b strings.Builder
	b.WriteString("Headers: ")
	b.WriteString(strings.Join(in.Headers, ", "))
	b.WriteString("\n\nSample rows:\n")
	const sampleCap = 5
	for i := range in.Rows { // bounded by len(in.Rows)
		if i >= sampleCap {
			break
		}
		row, err := json.Marshal(in.Rows[i])
		if err != nil {
			continue
		}
		b.Write(row)
		b.WriteByte('\n')
	}
	return b.String()
}
