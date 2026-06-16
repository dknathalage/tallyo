package agent

import (
	"context"
	"encoding/json"
	"fmt"

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
// directly onto repository.InvoiceInput / []repository.LineItemInput.
type createInvoiceInput struct {
	ParticipantID int64                      `json:"participantId"`
	PlanManagerID *int64                     `json:"planManagerId"`
	IssueDate     string                     `json:"issueDate"`
	DueDate       string                     `json:"dueDate"`
	Notes         string                     `json:"notes"`
	Items         []repository.LineItemInput `json:"items"`
}

// createInvoiceSchema is the model-facing JSON schema for create_invoice. The
// item shape mirrors repository.LineItemInput's json tags.
const createInvoiceSchema = `{
  "type": "object",
  "properties": {
    "participantId": { "type": "integer", "description": "Id of the participant the invoice is for." },
    "planManagerId": { "type": "integer", "description": "Optional plan manager id." },
    "issueDate": { "type": "string", "description": "Issue date (YYYY-MM-DD)." },
    "dueDate": { "type": "string", "description": "Due date (YYYY-MM-DD)." },
    "notes": { "type": "string", "description": "Optional notes." },
    "items": {
      "type": "array",
      "description": "Line items. For an NDIS support item supply code + serviceDate; for a custom line supply a description only.",
      "items": {
        "type": "object",
        "properties": {
          "code": { "type": "string", "description": "NDIS support item code (for catalogue lines)." },
          "description": { "type": "string" },
          "serviceDate": { "type": "string", "description": "Service date (YYYY-MM-DD); required for a support item." },
          "unit": { "type": "string" },
          "quantity": { "type": "number" },
          "unitPrice": { "type": "number" },
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

			header := repository.InvoiceInput{
				ParticipantID: in.ParticipantID,
				PlanManagerID: in.PlanManagerID,
				IssueDate:     in.IssueDate,
				DueDate:       in.DueDate,
				Notes:         in.Notes,
			}
			created, err := inv.Create(ctx, header, in.Items)
			if err != nil {
				if ve, ok := service.AsValidationError(err); ok {
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
