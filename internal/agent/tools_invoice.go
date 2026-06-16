package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/service"
)

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
