package smarts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/catalogue"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/session"
)

const dueDays = 14 // draft invoices default to net-14

// draftInvoiceSystem instructs the model: ground every line against the
// catalogue via search, never invent codes or prices, emit one line per distinct
// piece of billable work.
const draftInvoiceSystem = `You draft an invoice for a client from their unbilled work sessions.

Each session has a free-text note describing work done on a service date. Turn the
notes into invoice line items that map to the tenant's price-list catalogue.

Rules:
- Use the search tool to find the right catalogue item for each piece of work.
  Search by any field (code, name, category, unit). Never invent a code.
- Only emit a line for work you found a matching catalogue code for. Skip work
  with no catalogue match rather than guessing.
- You choose the code, quantity, unit, description and service date. You do NOT
  set prices — the system prices each line from the catalogue.
- When done, call draft_invoice with the final line list.`

// searchInput is the model's search-tool argument.
type searchInput struct {
	Query string `json:"query"`
}

// proposedLine is one line the model commits via the draft_invoice tool.
type proposedLine struct {
	Code        string  `json:"code"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	Quantity    float64 `json:"quantity"`
	ServiceDate string  `json:"serviceDate"`
}

type draftInvoiceCommit struct {
	Items []proposedLine `json:"items"`
}

// DraftInvoiceFromSessions gathers a client's unbilled sessions, grounds them
// against the catalogue via the model, and creates a DRAFT invoice priced
// deterministically from the catalogue. Returns the new invoice's uuid; the SPA
// navigates the user into the editable draft.
func (s *Service) DraftInvoiceFromSessions(ctx context.Context, clientUUID string) (string, error) {
	tenantID := reqctx.MustTenant(ctx)
	if clientUUID == "" {
		return "", fmt.Errorf("%w: client id required", ErrNotFound)
	}

	cl, err := s.clients.Get(ctx, clientUUID)
	if err != nil {
		return "", err
	}
	if cl == nil {
		return "", ErrNotFound
	}

	sessions, err := s.sessions.ListUnbilledForClient(ctx, tenantID, cl.ID)
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("%w: no unbilled sessions for this client", ErrNoData)
	}

	// Grounding search closure — tenant-scoped, all-fields, current catalogue.
	search := func(ctx context.Context, raw json.RawMessage) (string, error) {
		var in searchInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", err
		}
		items, err := s.cat.Search(ctx, tenantID, in.Query)
		if err != nil {
			return "", err
		}
		return encodeMatches(items), nil
	}

	user := buildDraftUser(sessions)
	commitJSON, err := s.llm.ProposeGrounded(ctx, GroundedRequest{
		System:     draftInvoiceSystem,
		User:       user,
		Search:     catalogueSearchTool,
		Commit:     draftInvoiceCommitTool,
		SearchFunc: search,
	})
	if err != nil {
		return "", err
	}

	var commit draftInvoiceCommit
	if err := json.Unmarshal(commitJSON, &commit); err != nil {
		return "", fmt.Errorf("smarts: parse draft: %w", err)
	}

	items := s.resolveLines(ctx, tenantID, commit.Items)
	if len(items) == 0 {
		return "", fmt.Errorf("%w: no catalogue lines could be drafted from these sessions", ErrNoData)
	}

	today := time.Now().UTC().Format("2006-01-02")
	due := time.Now().UTC().AddDate(0, 0, dueDays).Format("2006-01-02")
	inv, err := s.invoices.Create(ctx, invoice.InvoiceInput{
		ClientID:  cl.ID,
		Status:    "draft",
		IssueDate: today,
		DueDate:   due,
	}, items)
	if err != nil {
		return "", err
	}
	return inv.ID, nil
}

// resolveLines turns the model's proposed lines into priced LineItemInputs.
// Pricing is deterministic: each code is resolved against the current catalogue
// and the catalogue's unit_price/taxable are used; the line pins the catalogue
// version row via catalogueItemId. Unknown codes are dropped (the model was told
// to skip unmatched work). The model never sets a price.
func (s *Service) resolveLines(ctx context.Context, tenantID string, proposed []proposedLine) []billing.LineItemInput {
	out := make([]billing.LineItemInput, 0, len(proposed))
	for i := range proposed { // bounded by len(proposed)
		p := proposed[i]
		if p.Code == "" || p.Quantity <= 0 {
			continue
		}
		item, err := s.cat.GetCurrentByCode(ctx, tenantID, p.Code)
		if err != nil || item == nil {
			continue // unknown code — skip rather than guess
		}
		unit := p.Unit
		if unit == "" {
			unit = item.Unit
		}
		itemUUID := item.ID
		out = append(out, billing.LineItemInput{
			CatalogueItemID: &itemUUID,
			Code:            item.Code,
			Description:     p.Description,
			ServiceDate:     p.ServiceDate,
			Unit:            unit,
			Quantity:        p.Quantity,
			UnitPrice:       item.UnitPrice,
			Taxable:         item.Taxable,
			SortOrder:       int64(i),
		})
	}
	return out
}

// buildDraftUser renders the sessions into the user turn, fencing each note as
// untrusted content.
func buildDraftUser(sessions []*session.Session) string {
	var b strings.Builder
	b.WriteString("Unbilled sessions for this client:\n\n")
	for i := range sessions { // bounded by len(sessions)
		ss := sessions[i]
		fmt.Fprintf(&b, "Session on %s:\n%s\n\n", ss.ServiceDate, wrapUntrusted("session-note", ss.Note))
	}
	b.WriteString("Draft one invoice line per distinct piece of billable work, grounded against the catalogue.")
	return b.String()
}

// encodeMatches renders catalogue search results for the model (code/name/unit/
// category only — no internal ids). Capped to keep the tool result small.
func encodeMatches(items []*catalogue.CatalogueItem) string {
	const limit = 25
	type m struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Unit     string `json:"unit"`
		Category string `json:"category"`
	}
	out := make([]m, 0, len(items))
	for i := range items { // bounded by len(items)
		if i >= limit {
			break
		}
		out = append(out, m{Code: items[i].Code, Name: items[i].Name, Unit: items[i].Unit, Category: items[i].Category})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	if len(out) == 0 {
		return "no catalogue items matched that query"
	}
	return string(b)
}
