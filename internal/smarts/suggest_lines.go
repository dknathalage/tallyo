package smarts

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

const suggestLinesSystem = `You suggest invoice/estimate line items from a free-text note.

Turn the note into line items that map to the tenant's price-list catalogue.

Rules:
- Use the search tool to find the right catalogue item for each piece of work.
  Search by any field (code, name, category, unit). Never invent a code.
- Only suggest a line for work you found a matching catalogue code for.
- You choose the code, quantity, unit, description. You do NOT set prices — the
  system prices each line from the catalogue.
- When done, call suggest_lines with the line list.`

// SuggestInput is the request for the suggest-lines Smart: a free-text note and
// the service date that selects the catalogue version.
type SuggestInput struct {
	Note        string `json:"note"`
	ServiceDate string `json:"serviceDate"`
}

// SuggestLines turns a free-text note into catalogue-priced line items for the
// open invoice/estimate editor. It writes nothing — the SPA fills the editor and
// the normal save path validates. Pricing is deterministic from the catalogue.
func (s *Service) SuggestLines(ctx context.Context, in SuggestInput) ([]billing.LineItemInput, error) {
	tenantID := reqctx.MustTenant(ctx)
	if in.Note == "" {
		return nil, fmt.Errorf("%w: a note is required to suggest lines", ErrNoData)
	}
	date := in.ServiceDate
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	ver, err := s.cat.ResolveVersionForDate(ctx, tenantID, date)
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoPriceList, date)
	}

	search := func(ctx context.Context, raw json.RawMessage) (string, error) {
		var si searchInput
		if err := json.Unmarshal(raw, &si); err != nil {
			return "", err
		}
		items, err := s.cat.SearchItems(ctx, tenantID, ver.ID, si.Query)
		if err != nil {
			return "", err
		}
		return encodeMatches(items), nil
	}

	user := fmt.Sprintf("Note (service date %s):\n%s\n\nSuggest line items grounded against the catalogue.",
		date, wrapUntrusted("note", in.Note))
	commitJSON, err := s.llm.ProposeGrounded(ctx, GroundedRequest{
		System:     suggestLinesSystem,
		User:       user,
		Search:     catalogueSearchTool,
		Commit:     suggestLinesCommitTool,
		SearchFunc: search,
	})
	if err != nil {
		return nil, err
	}

	var commit draftInvoiceCommit
	if err := json.Unmarshal(commitJSON, &commit); err != nil {
		return nil, fmt.Errorf("smarts: parse suggestion: %w", err)
	}

	// Stamp each line's service date with the request date when the model omitted
	// it, then price deterministically from the catalogue.
	for i := range commit.Items { // bounded by len(commit.Items)
		if commit.Items[i].ServiceDate == "" {
			commit.Items[i].ServiceDate = date
		}
	}
	lines := s.resolveLines(ctx, tenantID, ver, commit.Items)
	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: nothing in the note matched the catalogue", ErrNoData)
	}
	return lines, nil
}
