package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/invoice"
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
func NewListInvoicesTool(inv InvoiceLister) Tool {
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

// NewCreateInvoiceTool returns a RISKY tool that creates an invoice for a
// participant with line items. Every line passes through the NDIS validation
// engine; a validation failure is returned as a structured tool error (not a
// panic) so the caller feeds an is_error tool_result. On success it records the
// create under the active checkpoint (if any, threaded via context) in a
// SEPARATE audited transaction, after the service write committed (B1).
func NewCreateInvoiceTool(inv InvoiceCreator, cp *Checkpoint) Tool {
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
func NewCreateInvoiceToolForShifts(inv InvoiceCreator, shifts ShiftWorker, cp *Checkpoint) Tool {
	return newCreateInvoiceToolShifts(inv, shifts, cp)
}

func newCreateInvoiceTool(inv InvoiceCreator, cp *Checkpoint) Tool {
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

			header := invoice.InvoiceInput{
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
func newCreateInvoiceToolShifts(inv InvoiceCreator, shifts ShiftWorker, cp *Checkpoint) Tool {
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

			header := invoice.InvoiceInput{
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

// InvoiceRestoreFunc returns a RestoreFunc that reverts one recorded invoice
// change via the service layer. It conflict-checks the live row's version
// (UpdatedAt) against the captured EntityVersion before applying; a mismatch
// returns ErrConflict so the caller records and skips it.
func InvoiceRestoreFunc(inv InvoiceAccessor) RestoreFunc {
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
