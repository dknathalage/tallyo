package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// Untrusted-content seam (Task 11): Invoice records returned by these tools
// may contain free-text fields authored by end-users or imported from external
// sources — in particular Invoice.Notes and LineItem.Description. These fields
// are returned as part of the structured JSON payload (the model needs the full
// record for IDs and numeric values). The system prompt (SystemPrompt()) guards
// against treating embedded content as instructions. For future turns that
// surface individual notes/descriptions as standalone text (e.g. a "get_invoice"
// detail tool), pass them through wrapUntrusted("invoice-notes", ...) before
// including them in a text block so the model processes them as data.

// listInvoicesInput is the parsed input for the list_invoices tool.
// Status is optional; when empty all invoices are returned.
type listInvoicesInput struct {
	Status string `json:"status"`
}

// NewListInvoicesTool returns a read tool that lists the current tenant's
// invoices, optionally filtered by status. Valid statuses are: draft, sent,
// paid, overdue. Call this when the user asks to see, list, or look up invoices.
func NewListInvoicesTool(inv *service.InvoiceService) Tool {
	return Tool{
		Name:        "list_invoices",
		Description: "List the current tenant's invoices. Optionally filter by status (draft|sent|paid|overdue). Call this when the user asks to see, list, or look up invoices.",
		Risk:        RiskRead,
		Render:      "table",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "description": "Optional status filter. One of: draft, sent, paid, overdue.",
      "enum": ["draft", "sent", "paid", "overdue"]
    }
  },
  "additionalProperties": false
}`),
		Handler: func(ctx context.Context, input json.RawMessage) (Result, error) {
			var in listInvoicesInput
			if err := json.Unmarshal(input, &in); err != nil {
				return Result{}, fmt.Errorf("list_invoices: invalid input: %w", err)
			}
			if in.Status != "" {
				rows, err := inv.ListByStatus(ctx, in.Status)
				if err != nil {
					return Result{}, fmt.Errorf("list_invoices: %w", err)
				}
				return Result{JSON: rows, Render: "table"}, nil
			}
			rows, err := inv.List(ctx)
			if err != nil {
				return Result{}, fmt.Errorf("list_invoices: %w", err)
			}
			return Result{JSON: rows, Render: "table"}, nil
		},
	}
}

// createInvoiceInput is the parsed input for the create_invoice tool. It maps
// directly onto repository.InvoiceInput / []billing.LineItemInput.
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

// NewCreateInvoiceTool returns a RISKY tool that creates an invoice for a
// participant with line items. Every line passes through the NDIS validation
// engine; a validation failure is returned as a structured tool error (not a
// panic) so the caller feeds an is_error tool_result. On success it records the
// create under the active checkpoint (if any, threaded via context) in a
// SEPARATE audited transaction, after the service write committed (B1).
func NewCreateInvoiceTool(inv *service.InvoiceService, cp *Checkpoint) Tool {
	return newCreateInvoiceTool(inv, cp)
}

// NewCreateInvoiceToolForShifts is NewCreateInvoiceTool plus a shift-completeness
// guard keyed on the participant's RECORDED shifts (the shifts lifecycle path).
// Before persisting, it checks that every quantity recorded on an unbilled shift
// in the invoice's date range is billed as a catalogue-CODED line with a matching
// quantity; a gap is returned as a structured is_error so the model self-corrects.
// On success it records the create under the checkpoint, then MarkDrafted the
// covered shifts (status → drafted, linked to the new invoice). Same create_invoice
// semantics otherwise (catalogue-authoritative pricing, RiskRisky, quantity guard).
func NewCreateInvoiceToolForShifts(inv *service.InvoiceService, shifts *service.ShiftService, cp *Checkpoint) Tool {
	return newCreateInvoiceToolShifts(inv, shifts, cp)
}

func newCreateInvoiceTool(inv *service.InvoiceService, cp *Checkpoint) Tool {
	return Tool{
		Name:        "create_invoice",
		Description: "Create a new invoice for a participant with line items. This is a write — it requires user approval before running.",
		Risk:        RiskRisky,
		Render:      "card",
		Schema:      json.RawMessage(createInvoiceSchema),
		Handler: func(ctx context.Context, input json.RawMessage) (Result, error) {
			var in createInvoiceInput
			if err := json.Unmarshal(input, &in); err != nil {
				return Result{}, fmt.Errorf("create_invoice: invalid input: %w", err)
			}
			if in.ParticipantID <= 0 {
				return Result{}, fmt.Errorf("create_invoice: participantId must be a positive integer")
			}
			if len(in.Items) == 0 {
				return Result{}, fmt.Errorf("create_invoice: at least one line item is required")
			}
			// A catalogue-coded line must carry a positive quantity: the platform
			// supplies the price, so a zero/absent quantity would silently bill $0.
			for i := range in.Items { // bounded by len(in.Items)
				it := in.Items[i]
				if it.Code != "" && it.Quantity <= 0 {
					return Result{}, fmt.Errorf("create_invoice: line %d (code %q) needs a quantity greater than 0", i, it.Code)
				}
			}

			header := repository.InvoiceInput{
				ParticipantID: in.ParticipantID,
				PlanManagerID: in.PlanManagerID,
				IssueDate:     in.IssueDate,
				DueDate:       in.DueDate,
				Notes:         in.Notes,
			}
			// Catalogue-authoritative pricing: the model chooses the code, service
			// date and quantity; the platform resolves the NDIS price. The model
			// cannot misprice a coded line (Pillar 1).
			created, err := inv.CreateWithCatalogPricing(ctx, header, in.Items)
			if err != nil {
				if ve, ok := billing.AsValidationError(err); ok {
					return Result{}, fmt.Errorf("create_invoice: invoice failed NDIS validation: %s", ve.Error())
				}
				return Result{}, fmt.Errorf("create_invoice: %w", err)
			}

			// Record the create under the active checkpoint (B1: separate tx,
			// after the service commit). No checkpoint in ctx → skip recording.
			if checkpointID, ok := checkpointFrom(ctx); ok && cp != nil {
				after, mErr := json.Marshal(created)
				if mErr != nil {
					return Result{}, fmt.Errorf("create_invoice: encode checkpoint row: %w", mErr)
				}
				if rErr := cp.Record(ctx, checkpointID, Change{
					Table: "invoices", PK: created.ID, Op: "create",
					AfterRow: after, EntityVersion: created.UpdatedAt,
				}); rErr != nil {
					return Result{}, fmt.Errorf("create_invoice: record checkpoint: %w", rErr)
				}
			}
			return Result{JSON: created, Render: "card"}, nil
		},
	}
}

// newCreateInvoiceToolShifts builds the create_invoice tool keyed on the
// participant's RECORDED shifts. It mirrors newCreateInvoiceTool but uses the
// shift verify/bill: before persisting it checks the draft covers every quantity
// recorded on an unbilled shift in [from, to] as a catalogue-CODED line, and on
// success links the covered shifts to the invoice (status → drafted).
func newCreateInvoiceToolShifts(inv *service.InvoiceService, shifts *service.ShiftService, cp *Checkpoint) Tool {
	return Tool{
		Name:        "create_invoice",
		Description: "Create a new invoice for a participant with line items. This is a write — it requires user approval before running.",
		Risk:        RiskRisky,
		Render:      "card",
		Schema:      json.RawMessage(createInvoiceSchema),
		Handler: func(ctx context.Context, input json.RawMessage) (Result, error) {
			var in createInvoiceInput
			if err := json.Unmarshal(input, &in); err != nil {
				return Result{}, fmt.Errorf("create_invoice: invalid input: %w", err)
			}
			if in.ParticipantID <= 0 {
				return Result{}, fmt.Errorf("create_invoice: participantId must be a positive integer")
			}
			if len(in.Items) == 0 {
				return Result{}, fmt.Errorf("create_invoice: at least one line item is required")
			}
			// A catalogue-coded line must carry a positive quantity: the platform
			// supplies the price, so a zero/absent quantity would silently bill $0.
			for i := range in.Items { // bounded by len(in.Items)
				it := in.Items[i]
				if it.Code != "" && it.Quantity <= 0 {
					return Result{}, fmt.Errorf("create_invoice: line %d (code %q) needs a quantity greater than 0", i, it.Code)
				}
			}
			// Completeness verify over recorded shifts. The coverage range is the
			// model-supplied [from, to] when present (catches a whole day the model
			// dropped), else derived from the coded lines. Pre-persist, so a gap
			// never leaves an orphan invoice.
			coverFrom, coverTo := in.From, in.To
			if shifts != nil {
				if err := verifyShiftsCovered(ctx, shifts, in.ParticipantID, in.Items, coverFrom, coverTo); err != nil {
					return Result{}, err
				}
			}

			header := repository.InvoiceInput{
				ParticipantID: in.ParticipantID,
				PlanManagerID: in.PlanManagerID,
				IssueDate:     in.IssueDate,
				DueDate:       in.DueDate,
				Notes:         in.Notes,
			}
			// Catalogue-authoritative pricing: the model chooses the code, service
			// date and quantity; the platform resolves the NDIS price (Pillar 1).
			created, err := inv.CreateWithCatalogPricing(ctx, header, in.Items)
			if err != nil {
				if ve, ok := billing.AsValidationError(err); ok {
					return Result{}, fmt.Errorf("create_invoice: invoice failed NDIS validation: %s", ve.Error())
				}
				return Result{}, fmt.Errorf("create_invoice: %w", err)
			}

			// Record the create under the active checkpoint (B1: separate tx, after
			// the service commit). No checkpoint in ctx → skip recording.
			if checkpointID, ok := checkpointFrom(ctx); ok && cp != nil {
				after, mErr := json.Marshal(created)
				if mErr != nil {
					return Result{}, fmt.Errorf("create_invoice: encode checkpoint row: %w", mErr)
				}
				if rErr := cp.Record(ctx, checkpointID, Change{
					Table: "invoices", PK: created.ID, Op: "create",
					AfterRow: after, EntityVersion: created.UpdatedAt,
				}); rErr != nil {
					return Result{}, fmt.Errorf("create_invoice: record checkpoint: %w", rErr)
				}
			}

			// Link the covered shifts to the new invoice (status → drafted).
			// Best-effort: a billing-link failure must not undo a committed invoice.
			if shifts != nil {
				billCoveredShifts(ctx, shifts, in.ParticipantID, created.ID, coverFrom, coverTo, in.Items)
			}
			return Result{JSON: created, Render: "card"}, nil
		},
	}
}

// verifyShiftsCovered checks that every quantity recorded on the participant's
// unbilled shifts for the draft's service-date range is billed as a
// catalogue-CODED line with a matching quantity (Pillar 4). A
// gap — a missing line, or a quantity billed as a custom line instead of an NDIS
// code — yields a structured error so the model self-corrects.
func verifyShiftsCovered(ctx context.Context, shifts *service.ShiftService, participantID int64, items []billing.LineItemInput, from, to string) error {
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
func billCoveredShifts(ctx context.Context, shifts *service.ShiftService, participantID, invoiceID int64, from, to string, items []billing.LineItemInput) {
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

// InvoiceRestoreFunc returns a RestoreFunc that reverts one recorded invoice
// change via the service layer. It conflict-checks the live row's version
// (UpdatedAt) against the captured EntityVersion before applying; a mismatch
// returns ErrConflict so the caller records and skips it.
func InvoiceRestoreFunc(inv *service.InvoiceService) RestoreFunc {
	return func(ctx context.Context, ch Change) error {
		if inv == nil {
			return fmt.Errorf("invoice restore: nil service")
		}
		if ch.Table != "invoices" {
			return fmt.Errorf("invoice restore: unexpected table %q", ch.Table)
		}
		live, err := inv.Get(ctx, ch.PK)
		if err != nil {
			return fmt.Errorf("invoice restore: load live: %w", err)
		}
		if live == nil {
			// Already gone: a create whose row no longer exists is a no-op.
			if ch.Op == "create" {
				return nil
			}
			return fmt.Errorf("invoice restore: invoice %d not found", ch.PK)
		}
		if live.UpdatedAt != ch.EntityVersion {
			return fmt.Errorf("invoice %d: %w", ch.PK, ErrConflict)
		}
		switch ch.Op {
		case "create":
			if e := inv.Delete(ctx, ch.PK); e != nil {
				return fmt.Errorf("invoice restore: delete: %w", e)
			}
			return nil
		case "update":
			return fmt.Errorf("invoice restore: update-revert is not supported in v1")
		default:
			return fmt.Errorf("invoice restore: unknown op %q", ch.Op)
		}
	}
}
