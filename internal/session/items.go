package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
)

// SetInvoice links a session to an invoice and sets its status, atomically.
func (r *SessionsRepo) SetInvoice(ctx context.Context, tenantID, id, invoiceID string, status string) error {
	if invoiceID == "" {
		return errors.New("set session invoice: invoice id required")
	}
	if status == "" {
		return errors.New("set session invoice: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "session", EntityID: id, Action: "bill",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID, "status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).SetSessionInvoice(ctx, gen.SetSessionInvoiceParams{
			InvoiceID: sql.NullString{String: invoiceID, Valid: true}, Status: status,
			UpdatedAt: now, TenantID: tenantID, ID: id,
		}); err != nil {
			return fmt.Errorf("set invoice: %w", err)
		}
		return nil
	})
}

// SetStatusForInvoice sets the status of every session linked to an invoice (e.g.
// cascading 'sent'/'paid' from the invoice), atomically.
func (r *SessionsRepo) SetStatusForInvoice(ctx context.Context, tenantID, invoiceID string, status string) error {
	if invoiceID == "" {
		return errors.New("set status for invoice: invoice id required")
	}
	if status == "" {
		return errors.New("set status for invoice: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "session", EntityID: "", Action: "status",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID, "status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).SetStatusForInvoice(ctx, gen.SetStatusForInvoiceParams{
			Status: status, UpdatedAt: now, TenantID: tenantID,
			InvoiceID: sql.NullString{String: invoiceID, Valid: true},
		}); err != nil {
			return fmt.Errorf("set status for invoice: %w", err)
		}
		return nil
	})
}

// ClearForInvoice reverts every session linked to an invoice back to 'recorded'
// with a NULL invoice_id (used when the invoice is deleted), atomically.
func (r *SessionsRepo) ClearForInvoice(ctx context.Context, tenantID, invoiceID string) error {
	if invoiceID == "" {
		return errors.New("clear sessions for invoice: invoice id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "session", EntityID: "", Action: "unbill",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).ClearSessionsForInvoice(ctx, gen.ClearSessionsForInvoiceParams{
			UpdatedAt: now, TenantID: tenantID,
			InvoiceID: sql.NullString{String: invoiceID, Valid: true},
		}); err != nil {
			return fmt.Errorf("clear for invoice: %w", err)
		}
		return nil
	})
}

// ListItems returns a session's line items (billed and unbilled), oldest first.
func (r *SessionsRepo) ListItems(ctx context.Context, tenantID, sessionID string) ([]*billing.LineItem, error) {
	if tenantID == "" || sessionID == "" {
		return nil, errors.New("list session items: tenant and session id required")
	}
	rows, err := gen.New(r.db).ListLineItemsForSession(ctx, gen.ListLineItemsForSessionParams{
		TenantID: tenantID, SessionID: sql.NullString{String: sessionID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list session items: %w", err)
	}
	out := make([]*billing.LineItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, billing.LineItemFromRow(billing.LineItemRowFromSessionList(rows[i])))
	}
	return out, nil
}

// GetItem returns one line item by id, or (nil, nil) when absent for the tenant.
func (r *SessionsRepo) GetItem(ctx context.Context, tenantID, itemID string) (*billing.LineItem, error) {
	if tenantID == "" || itemID == "" {
		return nil, errors.New("get session item: tenant and item id required")
	}
	row, err := gen.New(r.db).GetLineItem(ctx, gen.GetLineItemParams{TenantID: tenantID, ID: itemID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session item: %w", err)
	}
	return billing.LineItemFromRow(billing.LineItemRowFromGet(row)), nil
}

// CountItems returns how many UNBILLED items the session carries.
func (r *SessionsRepo) CountItems(ctx context.Context, tenantID, sessionID string) (int64, error) {
	if tenantID == "" || sessionID == "" {
		return 0, errors.New("count session items: tenant and session id required")
	}
	n, err := gen.New(r.db).CountSessionItems(ctx, gen.CountSessionItemsParams{
		TenantID: tenantID, SessionID: sql.NullString{String: sessionID, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("count session items: %w", err)
	}
	return n, nil
}

// CreateItem inserts a line item on a session (session_id set, invoice_id NULL) and
// writes one audit row. in is expected pre-priced by the caller.
func (r *SessionsRepo) CreateItem(ctx context.Context, tenantID, sessionID string, in billing.LineItemInput) (*billing.LineItem, error) {
	if tenantID == "" || sessionID == "" {
		return nil, errors.New("create session item: tenant and session id required")
	}
	if in.Quantity < 0 {
		return nil, errors.New("create session item: quantity must not be negative")
	}
	var newID string
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: sessionID, Action: "create",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		catalogueItemID, e := billing.ResolveCatalogueItemID(ctx, q, tenantID, in.CatalogueItemID)
		if e != nil {
			return e
		}
		row, e := q.CreateLineItem(ctx, lineItemParams(tenantID, &sessionID, catalogueItemID, in))
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newID = row.ID
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create session item: %w", err)
	}
	return r.GetItem(ctx, tenantID, newID)
}

// UpdateItem rewrites an UNBILLED session item (invoice_id IS NULL guard) and
// writes one audit row. Returns (nil, nil) when the item is absent or already
// billed. in is expected pre-priced by the caller.
func (r *SessionsRepo) UpdateItem(ctx context.Context, tenantID, itemID string, in billing.LineItemInput) (*billing.LineItem, error) {
	if tenantID == "" || itemID == "" {
		return nil, errors.New("update session item: tenant and item id required")
	}
	if in.Quantity < 0 {
		return nil, errors.New("update session item: quantity must not be negative")
	}
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: itemID, Action: "update",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		catalogueItemID, e := billing.ResolveCatalogueItemID(ctx, q, tenantID, in.CatalogueItemID)
		if e != nil {
			return e
		}
		_, e = q.UpdateSessionLineItem(ctx, gen.UpdateSessionLineItemParams{
			CatalogueItemID: catalogueItemID,
			Code:            db.NzMaybe(in.Code),
			Description:     in.Description,
			ServiceDate:     db.NzMaybe(in.ServiceDate),
			Unit:            db.NzMaybe(in.Unit),
			StartTime:       db.NzMaybe(in.StartTime),
			EndTime:         db.NzMaybe(in.EndTime),
			Quantity:        in.Quantity,
			UnitPrice:       in.UnitPrice,
			Taxable:         db.B2i(in.Taxable),
			LineTotal:       billing.Round2(in.Quantity * in.UnitPrice),
			TenantID:        tenantID,
			ID:              itemID,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return nil
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update session item: %w", err)
	}
	return r.GetItem(ctx, tenantID, itemID)
}

// DeleteUnbilledItems removes ALL of a session's unbilled items (invoice_id IS
// NULL) in one audited mutation. Used to make a re-divide idempotent.
func (r *SessionsRepo) DeleteUnbilledItems(ctx context.Context, tenantID, sessionID string) error {
	if tenantID == "" || sessionID == "" {
		return errors.New("delete unbilled items: tenant and session id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: sessionID, Action: "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteUnbilledItemsForSession(ctx, gen.DeleteUnbilledItemsForSessionParams{
			TenantID: tenantID, SessionID: sql.NullString{String: sessionID, Valid: true},
		}); err != nil {
			return fmt.Errorf("delete unbilled items: %w", err)
		}
		return nil
	})
}

// DeleteItem removes an UNBILLED session item (invoice_id IS NULL guard) and writes
// one audit row.
func (r *SessionsRepo) DeleteItem(ctx context.Context, tenantID, itemID string) error {
	if tenantID == "" || itemID == "" {
		return errors.New("delete session item: tenant and item id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: itemID, Action: "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteSessionLineItem(ctx, gen.DeleteSessionLineItemParams{TenantID: tenantID, ID: itemID}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// GetItemByUUID returns a session's line item addressed by uuid, scoped to the
// owning session's row id, or (nil, nil) when absent. The session scope ensures an
// item uuid from another session (or tenant) 404s.
func (r *SessionsRepo) GetItemByUUID(ctx context.Context, tenantID, sessionID string, itemUUID string) (*billing.LineItem, error) {
	if tenantID == "" || sessionID == "" {
		return nil, errors.New("get session item: tenant and session id required")
	}
	row, err := gen.New(r.db).GetSessionLineItemByUUID(ctx, gen.GetSessionLineItemByUUIDParams{
		TenantID: tenantID, SessionID: sql.NullString{String: sessionID, Valid: true}, ID: itemUUID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session item by uuid: %w", err)
	}
	return billing.LineItemFromRow(billing.LineItemRowFromSessionUUID(row)), nil
}

// UpdateItemByUUID rewrites an UNBILLED session item addressed by uuid (scoped to
// the owning session, invoice_id IS NULL guard) and writes one audit row. Returns
// (nil, nil) when the item is absent or already billed. in is expected
// pre-priced by the caller. The audit EntityID keeps the item's row id (uuid).
func (r *SessionsRepo) UpdateItemByUUID(ctx context.Context, tenantID, sessionID string, itemUUID string, in billing.LineItemInput) (*billing.LineItem, error) {
	if tenantID == "" || sessionID == "" {
		return nil, errors.New("update session item: tenant and session id required")
	}
	if in.Quantity < 0 {
		return nil, errors.New("update session item: quantity must not be negative")
	}
	var item *billing.LineItem
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		catalogueItemID, e := billing.ResolveCatalogueItemID(ctx, q, tenantID, in.CatalogueItemID)
		if e != nil {
			return e
		}
		row, e := q.UpdateSessionLineItemByUUID(ctx, gen.UpdateSessionLineItemByUUIDParams{
			CatalogueItemID: catalogueItemID,
			Code:            db.NzMaybe(in.Code),
			Description:     in.Description,
			ServiceDate:     db.NzMaybe(in.ServiceDate),
			Unit:            db.NzMaybe(in.Unit),
			StartTime:       db.NzMaybe(in.StartTime),
			EndTime:         db.NzMaybe(in.EndTime),
			Quantity:        in.Quantity,
			UnitPrice:       in.UnitPrice,
			Taxable:         db.B2i(in.Taxable),
			LineTotal:       billing.Round2(in.Quantity * in.UnitPrice),
			TenantID:        tenantID,
			SessionID:       sql.NullString{String: sessionID, Valid: true},
			ID:              itemUUID,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		item = billing.LineItemFromRow(lineItemRowFromGen(row, in.CatalogueItemID))
		return audit.Log(ctx, tx, audit.Entry{EntityType: "line_item", EntityID: row.ID, Action: "update"})
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update session item by uuid: %w", err)
	}
	return item, nil
}

// DeleteItemByUUID removes an UNBILLED session item addressed by uuid (scoped to
// the owning session, invoice_id IS NULL guard) and writes one audit row. A
// missing/billed item is a no-op. The audit EntityID keeps the item's row id (uuid),
// resolved in-tx.
func (r *SessionsRepo) DeleteItemByUUID(ctx context.Context, tenantID, sessionID string, itemUUID string) error {
	if tenantID == "" || sessionID == "" {
		return errors.New("delete session item: tenant and session id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetSessionLineItemByUUID(ctx, gen.GetSessionLineItemByUUIDParams{
			TenantID: tenantID, SessionID: sql.NullString{String: sessionID, Valid: true}, ID: itemUUID,
		})
		if errors.Is(e, sql.ErrNoRows) {
			return nil // missing → no-op
		}
		if e != nil {
			return fmt.Errorf("resolve item: %w", e)
		}
		if err := q.DeleteSessionLineItemByUUID(ctx, gen.DeleteSessionLineItemByUUIDParams{
			TenantID: tenantID, SessionID: sql.NullString{String: sessionID, Valid: true}, ID: itemUUID,
		}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{EntityType: "line_item", EntityID: row.ID, Action: "delete"})
	})
}

// lineItemRowFromGen adapts a bare gen.LineItem (an UPDATE/INSERT RETURNING *
// row, which carries no catalogue join) into a billing.LineItemRow, stamping the
// catalogue uuid from the resolved inbound value so catalogueItemId round-trips
// without a re-read.
func lineItemRowFromGen(r gen.LineItem, catalogueItemUUID *string) billing.LineItemRow {
	return billing.LineItemRow{
		ID: r.ID, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		CatalogueItemID: r.CatalogueItemID, CatalogueItemUuid: db.NullStr(catalogueItemUUID),
		Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

// lineItemParams builds the gen insert params for a line item. sessionID nil = an
// invoice-only line; here it is always set (session item, invoice_id NULL). The
// inbound custom-item uuid is resolved to the row id (uuid) by the caller and passed in.
func lineItemParams(tenantID string, sessionID *string, catalogueItemID sql.NullString, in billing.LineItemInput) gen.CreateLineItemParams {
	return gen.CreateLineItemParams{
		ID:              ids.New(),
		TenantID:        tenantID,
		SessionID:       db.NullStr(sessionID),
		InvoiceID:       sql.NullString{}, // unbilled session item
		CatalogueItemID: catalogueItemID,
		Code:            db.NzMaybe(in.Code),
		Description:     in.Description,
		ServiceDate:     db.NzMaybe(in.ServiceDate),
		Unit:            db.NzMaybe(in.Unit),
		StartTime:       db.NzMaybe(in.StartTime),
		EndTime:         db.NzMaybe(in.EndTime),
		Quantity:        in.Quantity,
		UnitPrice:       in.UnitPrice,
		Taxable:         db.B2i(in.Taxable),
		LineTotal:       billing.Round2(in.Quantity * in.UnitPrice),
		SortOrder:       sql.NullInt64{Int64: in.SortOrder, Valid: true},
	}
}
