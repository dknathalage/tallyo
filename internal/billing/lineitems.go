package billing

// Shared line_items insert. Both the invoice slice (create/update) and the
// recurring slice (generation) write the line_items table with identical column
// mapping, so the insert lives here rather than in either slice. It serves the
// line_items table ONLY; estimate_line_items has a different column set and keeps
// estimate.copyEstimateItemsToInvoice.

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
)

// InsertLineItems writes each line item with its computed total. Bounded by len.
// Shared by the invoice and recurring repositories (the line_items table).
func InsertLineItems(ctx context.Context, q *gen.Queries, tenantID, invoiceID string, items []LineItemInput) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		customItemID, err := ResolveCustomItemID(ctx, q, tenantID, it.CustomItemID)
		if err != nil {
			return fmt.Errorf("insert line item %d: %w", i, err)
		}
		_, err = q.CreateLineItem(ctx, gen.CreateLineItemParams{
			ID:                 ids.New(),
			TenantID:           tenantID,
			SessionID:          sql.NullString{}, // invoice lines from this path are not session items
			InvoiceID:          sql.NullString{String: invoiceID, Valid: true},
			ItemID:             db.NullStr(it.ItemID),
			CustomItemID:       customItemID,
			PriceListVersionID: db.NullStr(it.PriceListVersionID),
			Code:               db.NzMaybe(it.Code),
			Description:        it.Description,
			ServiceDate:        db.NzMaybe(it.ServiceDate),
			Unit:               db.NzMaybe(it.Unit),
			StartTime:          db.NzMaybe(it.StartTime),
			EndTime:            db.NzMaybe(it.EndTime),
			Quantity:           it.Quantity,
			UnitPrice:          it.UnitPrice,
			Taxable:            db.B2i(it.Taxable),
			LineTotal:          Round2(it.Quantity * it.UnitPrice),
			SortOrder:          sql.NullInt64{Int64: it.SortOrder, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("insert line item %d: %w", i, err)
		}
	}
	return nil
}
