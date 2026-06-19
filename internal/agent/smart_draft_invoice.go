package agent

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/invoice"
)

// createInvoiceInput is the parsed input for the create_invoice tool. It maps
// directly onto invoice.InvoiceInput / []billing.LineItemInput.
type createInvoiceInput struct {
	ParticipantID int64  `json:"participantId"`
	PlanManagerID *int64 `json:"planManagerId"`
	IssueDate     string `json:"issueDate"`
	DueDate       string `json:"dueDate"`
	Notes         string `json:"notes"`
	// From/To are the inclusive service-date window of recorded shifts this
	// invoice covers.
	// Passed on the shifts→invoice path so the platform can verify completeness
	// and mark the covered shifts drafted.
	From  string                  `json:"from"`
	To    string                  `json:"to"`
	Items []billing.LineItemInput `json:"items"`
}

// createInvoiceSchema is the model-facing JSON schema for create_invoice. The
// item shape mirrors billing.LineItemInput's json tags.
const createInvoiceSchema = `{
  "type": "object",
  "properties": {
    "participantId": { "type": "integer", "description": "Id of the participant the invoice is for." },
    "planManagerId": { "type": "integer", "description": "Optional plan manager id." },
    "issueDate": { "type": "string", "description": "Issue date (YYYY-MM-DD)." },
    "dueDate": { "type": "string", "description": "Due date (YYYY-MM-DD)." },
    "notes": { "type": "string", "description": "Optional notes." },
    "from": { "type": "string", "description": "When drafting from a participant's recorded shifts: the inclusive start (YYYY-MM-DD) of the shift range this invoice covers. Pass the same range you read shifts for, so the platform can verify every recorded shift is billed and link the shifts." },
    "to": { "type": "string", "description": "Inclusive end (YYYY-MM-DD) of the covered shift range. Pass alongside from." },
    "items": {
      "type": "array",
      "description": "Line items. For an NDIS support item supply code + serviceDate + quantity and OMIT unitPrice — the platform applies the authoritative NDIS price for that code, date and zone. For a custom line supply a description + quantity + unitPrice.",
      "items": {
        "type": "object",
        "properties": {
          "code": { "type": "string", "description": "NDIS support item code (for catalogue lines)." },
          "description": { "type": "string" },
          "serviceDate": { "type": "string", "description": "Service date (YYYY-MM-DD); required for a support item." },
          "unit": { "type": "string" },
          "quantity": { "type": "number", "description": "Billable quantity (hours, km, each); must be greater than 0." },
          "unitPrice": { "type": "number", "description": "Custom-line price only. IGNORED for a catalogue code (the platform applies the NDIS price)." },
          "gstFree": { "type": "boolean" },
          "sortOrder": { "type": "integer" }
        },
        "additionalProperties": false
      }
    }
  },
  "required": ["participantId", "items"],
  "additionalProperties": false
}`

// applyDraftInvoice is the deterministic half of the draft-invoice Smart: it
// validates the model's proposal, verifies it covers every recorded shift in the
// window, prices coded lines from the catalogue, persists the invoice, and links
// the covered shifts (status → drafted). No checkpoint recording — the draft is
// itself the reviewable artifact. Returns a recoverable (model-fixable) error for
// validation/coverage failures so a retry can self-correct.
func (s *Smarts) applyDraftInvoice(ctx context.Context, in createInvoiceInput) (*invoice.Invoice, error) {
	if in.ParticipantID <= 0 {
		return nil, fmt.Errorf("draft invoice: participantId must be a positive integer")
	}
	if len(in.Items) == 0 {
		return nil, fmt.Errorf("draft invoice: at least one line item is required")
	}
	for i := range in.Items { // bounded by len(in.Items)
		it := in.Items[i]
		if it.Code != "" && it.Quantity <= 0 {
			return nil, fmt.Errorf("draft invoice: line %d (code %q) needs a quantity greater than 0", i, it.Code)
		}
	}

	coverFrom, coverTo := in.From, in.To
	if err := verifyShiftsCovered(ctx, s.shifts, in.ParticipantID, in.Items, coverFrom, coverTo); err != nil {
		return nil, err // already prefixed; recoverable (coverage gap)
	}

	header := invoice.InvoiceInput{
		ParticipantID: in.ParticipantID,
		PlanManagerID: in.PlanManagerID,
		IssueDate:     in.IssueDate,
		DueDate:       in.DueDate,
		Notes:         in.Notes,
	}
	created, err := s.invoice.CreateWithCatalogPricing(ctx, header, in.Items)
	if err != nil {
		if ve, ok := billing.AsValidationError(err); ok {
			return nil, fmt.Errorf("draft invoice: invoice failed NDIS validation: %s", ve.Error())
		}
		return nil, fmt.Errorf("draft invoice: %w", err)
	}

	billCoveredShifts(ctx, s.shifts, in.ParticipantID, created.ID, coverFrom, coverTo, in.Items)
	return created, nil
}

// verifyShiftsCovered checks that every quantity recorded on the participant's
// unbilled shifts for the draft's service-date range is billed as a
// catalogue-CODED line with a matching quantity (Pillar 4). A
// gap — a missing line, or a quantity billed as a custom line instead of an NDIS
// code — yields a structured error so the model self-corrects.
func verifyShiftsCovered(ctx context.Context, shifts ShiftLister, participantID int64, items []billing.LineItemInput, from, to string) error {
	from, to, ok := coverageRange(items, from, to)
	if !ok {
		return nil // no coded lines and no explicit range → nothing to verify
	}
	recs, err := shifts.ListParticipant(ctx, participantID, from, to)
	if err != nil {
		return fmt.Errorf("create_invoice: verify shifts: %w", err)
	}
	gaps := make([]string, 0)
	for i := range recs { // bounded by len(recs)
		sh := recs[i]
		if sh.InvoiceID != nil {
			continue // already billed on another invoice
		}
		if round2c(sh.Km) > 0 && !hasCodedLine(items, sh.ServiceDate, sh.Km) {
			gaps = append(gaps, fmt.Sprintf("%s: transport %.2f km", sh.ServiceDate, sh.Km))
		}
		if round2c(sh.Hours) > 0 && !hasCodedLine(items, sh.ServiceDate, sh.Hours) {
			gaps = append(gaps, fmt.Sprintf("%s: support %.2f hours", sh.ServiceDate, sh.Hours))
		}
	}
	if len(gaps) > 0 {
		return fmt.Errorf("create_invoice: the draft does not cover every recorded shift. "+
			"For each item below, add a catalogue-CODED line on that date with that quantity "+
			"(use search_catalogue to find the NDIS code; do NOT bill it as a custom line): %s",
			strings.Join(gaps, "; "))
	}
	return nil
}

// billCoveredShifts links every unbilled shift in the coverage range to the new
// invoice (status → drafted via MarkDrafted). Best-effort: errors are swallowed
// (a failed link must not undo a committed invoice); a revert clears the link via
// FK. Bounded by the number of shifts in range.
func billCoveredShifts(ctx context.Context, shifts ShiftWorker, participantID, invoiceID int64, from, to string, items []billing.LineItemInput) {
	rf, rt, ok := coverageRange(items, from, to)
	if !ok {
		return
	}
	recs, err := shifts.ListParticipant(ctx, participantID, rf, rt)
	if err != nil {
		return
	}
	ids := make([]int64, 0, len(recs))
	for i := range recs { // bounded by len(recs)
		if recs[i].InvoiceID == nil {
			ids = append(ids, recs[i].ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	_ = shifts.MarkDrafted(ctx, invoiceID, ids)
}

// coverageRange returns the shift range to verify/bill against: the explicit
// [from, to] when both are supplied (catches a whole day the model dropped),
// otherwise the min/max over the coded lines.
func coverageRange(items []billing.LineItemInput, from, to string) (string, string, bool) {
	if from != "" && to != "" {
		return from, to, true
	}
	return codedDateRange(items)
}

// codedDateRange returns the min/max service date over the coded lines, and
// whether any coded line exists. Bounded by len(items).
func codedDateRange(items []billing.LineItemInput) (from, to string, ok bool) {
	for i := range items {
		if items[i].Code == "" || items[i].ServiceDate == "" {
			continue
		}
		d := items[i].ServiceDate
		if !ok {
			from, to, ok = d, d, true
			continue
		}
		if d < from {
			from = d
		}
		if d > to {
			to = d
		}
	}
	return from, to, ok
}

// hasCodedLine reports whether items contains a catalogue-coded line on
// serviceDate whose quantity equals qty (at cent granularity). Bounded by
// len(items).
func hasCodedLine(items []billing.LineItemInput, serviceDate string, qty float64) bool {
	for i := range items {
		it := items[i]
		if it.Code != "" && it.ServiceDate == serviceDate && round2c(it.Quantity) == round2c(qty) {
			return true
		}
	}
	return false
}

// round2c rounds to cents, matching the money/quantity rounding used elsewhere.
func round2c(x float64) float64 { return math.Round(x*100) / 100 }
