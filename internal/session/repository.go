// Package shift is the shift vertical slice: domain types, the audited
// repository over the shifts table, the service (with SSE broadcast), and the
// HTTP handler. It depends only on platform packages (db/gen, audit, reqctx,
// realtime, httpx), never on other domain slices.
package shift

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// Shift is the domain view of a row in the shifts table — the delivered-support
// unit a provider records for a client. A shift's billable quantities live
// on its line_items rows (see ListItems), not on the shift itself. Tags is
// stored as JSON TEXT and is never nil. Status moves through the lifecycle
// scheduled→recorded→drafted→sent→paid; InvoiceID is set once the shift is
// drafted onto an invoice.
type Shift struct {
	ID           int64    `json:"-"`
	UUID         string   `json:"id"`
	ClientID     int64    `json:"-"`
	ClientUUID   string   `json:"clientId"`
	ServiceDate  string   `json:"serviceDate"`
	Note         string   `json:"note"`
	Tags         []string `json:"tags"`
	Status       string   `json:"status"`
	InvoiceID    *int64   `json:"-"`         // internal FK; the public ref is invoiceId (the linked invoice's uuid)
	InvoiceUUID  *string  `json:"invoiceId"` // linked invoice uuid (nil until the shift is drafted onto an invoice)
	AuthorUserID *int64   `json:"-"`         // internal author user FK; not linked from the SPA
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// ShiftInput is the writable subset of a shift.
type ShiftInput struct {
	ClientID    int64    `json:"clientId"`
	ServiceDate string   `json:"serviceDate"`
	Note        string   `json:"note"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
}

// UnbilledAgg summarises a client's recorded-but-unbilled shifts: how many there
// are and the service-date span they cover.
type UnbilledAgg struct {
	ClientID int64  `json:"clientId"`
	Count    int64  `json:"count"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// ShiftsRepo reads and writes the shifts table (tenant-scoped) with audited
// mutations.
type ShiftsRepo struct {
	db db.Executor
}

// NewShifts constructs a repository. A nil db is a programmer error.
func NewShifts(db db.Executor) *ShiftsRepo {
	if db == nil {
		panic("shift: NewShifts requires a non-nil *sql.DB")
	}
	return &ShiftsRepo{db: db}
}

// Create inserts a shift and writes one audit row, atomically. authorUserID is
// the user the shift is attributed to (nil when unknown). Status defaults to
// 'recorded' when the input leaves it empty.
func (r *ShiftsRepo) Create(ctx context.Context, tenantID int64, authorUserID *int64, in ShiftInput) (*Shift, error) {
	if tenantID == 0 {
		return nil, errors.New("create shift: tenant id required")
	}
	if in.ClientID == 0 {
		return nil, errors.New("create shift: client id required")
	}
	if !validISODate(in.ServiceDate) {
		return nil, errors.New("create shift: service date must be a valid YYYY-MM-DD date")
	}
	tags, err := encodeTags(in.Tags)
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = "recorded"
	}

	var newID int64
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		s, e := gen.New(tx).CreateShift(ctx, gen.CreateShiftParams{
			Uuid:         uuid.NewString(),
			TenantID:     tenantID,
			ClientID:     in.ClientID,
			ServiceDate:  in.ServiceDate,
			Note:         in.Note,
			Tags:         tags,
			Status:       status,
			AuthorUserID: db.NullID(authorUserID),
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newID = s.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "shift", EntityID: s.ID, Action: "create",
			Changes: audit.Changes(map[string]any{"clientId": in.ClientID, "serviceDate": in.ServiceDate}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create shift: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// Get returns the tenant's shift by int PK, or (nil, nil) when absent. This is
// the internal/cross-slice read (agent ShiftReader, the service's own pricing
// path); the public HTTP path addresses shifts by uuid via GetByUUID.
func (r *ShiftsRepo) Get(ctx context.Context, tenantID, id int64) (*Shift, error) {
	row, err := gen.New(r.db).GetShiftByID(ctx, gen.GetShiftByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shift: %w", err)
	}
	return mapShift(shiftFieldsFromByID(row))
}

// GetByUUID returns the tenant's shift by uuid, or (nil, nil) when absent (or
// owned by another tenant — the query is tenant-scoped). This is the public
// HTTP read.
func (r *ShiftsRepo) GetByUUID(ctx context.Context, tenantID int64, shiftUUID string) (*Shift, error) {
	row, err := gen.New(r.db).GetShift(ctx, gen.GetShiftParams{TenantID: tenantID, Uuid: shiftUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shift by uuid: %w", err)
	}
	return mapShift(shiftFieldsFromGet(row))
}

// ResolveID translates a shift uuid into its int PK for the tenant. Returns
// (0, nil) when no such shift exists (so callers can 404 without an error).
func (r *ShiftsRepo) ResolveID(ctx context.Context, tenantID int64, shiftUUID string) (int64, error) {
	id, err := gen.New(r.db).GetShiftIDByUUID(ctx, gen.GetShiftIDByUUIDParams{TenantID: tenantID, Uuid: shiftUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve shift uuid: %w", err)
	}
	return id, nil
}

// ResolveClientID translates a client uuid into its int PK for the
// tenant (used by the ?client= shift filter and inbound clientId
// resolution). Returns (0, nil) when absent.
func (r *ShiftsRepo) ResolveClientID(ctx context.Context, tenantID int64, clientUUID string) (int64, error) {
	id, err := gen.New(r.db).GetClientIDByUUID(ctx, gen.GetClientIDByUUIDParams{TenantID: tenantID, Uuid: clientUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve client uuid: %w", err)
	}
	return id, nil
}

// ListClient returns a client's shifts. When both from and to are
// non-empty it restricts to service_date ∈ [from, to]; otherwise it returns all.
func (r *ShiftsRepo) ListClient(ctx context.Context, tenantID, clientID int64, from, to string) ([]*Shift, error) {
	if tenantID == 0 || clientID == 0 {
		return nil, errors.New("list shifts: tenant and client id required")
	}
	q := gen.New(r.db)
	if from != "" && to != "" {
		rows, err := q.ListShiftsByClientRange(ctx, gen.ListShiftsByClientRangeParams{
			TenantID: tenantID, ClientID: clientID, ServiceDate: from, ServiceDate_2: to,
		})
		if err != nil {
			return nil, fmt.Errorf("list client shifts range: %w", err)
		}
		return mapShifts(rows, shiftFieldsFromByPartRange)
	}
	rows, err := q.ListShiftsByClient(ctx, gen.ListShiftsByClientParams{
		TenantID: tenantID, ClientID: clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("list client shifts: %w", err)
	}
	return mapShifts(rows, shiftFieldsFromByPart)
}

// List returns all of the tenant's shifts (newest service date first).
func (r *ShiftsRepo) List(ctx context.Context, tenantID int64) ([]*Shift, error) {
	if tenantID == 0 {
		return nil, errors.New("list shifts: tenant id required")
	}
	rows, err := gen.New(r.db).ListShifts(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list shifts: %w", err)
	}
	return mapShifts(rows, shiftFieldsFromList)
}

// ListByStatus returns the tenant's shifts in a given lifecycle status.
func (r *ShiftsRepo) ListByStatus(ctx context.Context, tenantID int64, status string) ([]*Shift, error) {
	if tenantID == 0 {
		return nil, errors.New("list shifts by status: tenant id required")
	}
	rows, err := gen.New(r.db).ListShiftsByStatus(ctx, gen.ListShiftsByStatusParams{TenantID: tenantID, Status: status})
	if err != nil {
		return nil, fmt.Errorf("list shifts by status: %w", err)
	}
	return mapShifts(rows, shiftFieldsFromByStatus)
}

// ListScheduled returns the tenant's scheduled (not yet recorded) shifts.
func (r *ShiftsRepo) ListScheduled(ctx context.Context, tenantID int64) ([]*Shift, error) {
	if tenantID == 0 {
		return nil, errors.New("list scheduled shifts: tenant id required")
	}
	rows, err := gen.New(r.db).ListScheduledShifts(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list scheduled shifts: %w", err)
	}
	return mapShifts(rows, shiftFieldsFromScheduled)
}

// ListRecordedUnbilled returns a client's recorded shifts that are not yet
// linked to an invoice (status 'recorded', invoice_id NULL).
func (r *ShiftsRepo) ListRecordedUnbilled(ctx context.Context, tenantID, clientID int64) ([]*Shift, error) {
	if tenantID == 0 || clientID == 0 {
		return nil, errors.New("list recorded unbilled: tenant and client id required")
	}
	rows, err := gen.New(r.db).ListRecordedUnbilledByClient(ctx, gen.ListRecordedUnbilledByClientParams{
		TenantID: tenantID, ClientID: clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("list recorded unbilled: %w", err)
	}
	return mapShifts(rows, shiftFieldsFromRecorded)
}

// Update rewrites a shift's editable fields by uuid and writes one audit row,
// atomically. Returns (nil, nil) when the shift does not exist for the tenant.
// The audit EntityID keeps the int PK, recovered from the RETURNING row.
func (r *ShiftsRepo) Update(ctx context.Context, tenantID int64, shiftUUID string, in ShiftInput) (*Shift, error) {
	if !validISODate(in.ServiceDate) {
		return nil, errors.New("update shift: service date must be a valid YYYY-MM-DD date")
	}
	tags, err := encodeTags(in.Tags)
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = "recorded"
	}

	var missing bool
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		row, e := gen.New(tx).UpdateShift(ctx, gen.UpdateShiftParams{
			ServiceDate: in.ServiceDate,
			Note:        in.Note,
			Tags:        tags,
			Status:      status,
			UpdatedAt:   now,
			TenantID:    tenantID,
			Uuid:        shiftUUID,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{EntityType: "shift", EntityID: row.ID, Action: "update"})
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update shift: %w", err)
	}
	return r.GetByUUID(ctx, tenantID, shiftUUID)
}

// UpdateStatus sets a shift's lifecycle status by uuid and writes one audit row.
// The audit EntityID keeps the int PK, resolved in-tx; a missing row is a no-op.
func (r *ShiftsRepo) UpdateStatus(ctx context.Context, tenantID int64, shiftUUID, status string) error {
	if status == "" {
		return errors.New("update shift status: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		id, e := q.GetShiftIDByUUID(ctx, gen.GetShiftIDByUUIDParams{TenantID: tenantID, Uuid: shiftUUID})
		if errors.Is(e, sql.ErrNoRows) {
			return nil // missing row → silent no-op
		}
		if e != nil {
			return fmt.Errorf("resolve shift: %w", e)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if err := q.UpdateShiftStatus(ctx, gen.UpdateShiftStatusParams{
			Status: status, UpdatedAt: now, TenantID: tenantID, Uuid: shiftUUID,
		}); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "shift", EntityID: id, Action: "status",
			Changes: audit.Changes(map[string]any{"status": status}),
		})
	})
}

// SetInvoice links a shift to an invoice and sets its status, atomically.
func (r *ShiftsRepo) SetInvoice(ctx context.Context, tenantID, id, invoiceID int64, status string) error {
	if invoiceID == 0 {
		return errors.New("set shift invoice: invoice id required")
	}
	if status == "" {
		return errors.New("set shift invoice: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: id, Action: "bill",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID, "status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).SetShiftInvoice(ctx, gen.SetShiftInvoiceParams{
			InvoiceID: sql.NullInt64{Int64: invoiceID, Valid: true}, Status: status,
			UpdatedAt: now, TenantID: tenantID, ID: id,
		}); err != nil {
			return fmt.Errorf("set invoice: %w", err)
		}
		return nil
	})
}

// SetStatusForInvoice sets the status of every shift linked to an invoice (e.g.
// cascading 'sent'/'paid' from the invoice), atomically.
func (r *ShiftsRepo) SetStatusForInvoice(ctx context.Context, tenantID, invoiceID int64, status string) error {
	if invoiceID == 0 {
		return errors.New("set status for invoice: invoice id required")
	}
	if status == "" {
		return errors.New("set status for invoice: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: 0, Action: "status",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID, "status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).SetStatusForInvoice(ctx, gen.SetStatusForInvoiceParams{
			Status: status, UpdatedAt: now, TenantID: tenantID,
			InvoiceID: sql.NullInt64{Int64: invoiceID, Valid: true},
		}); err != nil {
			return fmt.Errorf("set status for invoice: %w", err)
		}
		return nil
	})
}

// ClearForInvoice reverts every shift linked to an invoice back to 'recorded'
// with a NULL invoice_id (used when the invoice is deleted), atomically.
func (r *ShiftsRepo) ClearForInvoice(ctx context.Context, tenantID, invoiceID int64) error {
	if invoiceID == 0 {
		return errors.New("clear shifts for invoice: invoice id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: 0, Action: "unbill",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).ClearShiftsForInvoice(ctx, gen.ClearShiftsForInvoiceParams{
			UpdatedAt: now, TenantID: tenantID,
			InvoiceID: sql.NullInt64{Int64: invoiceID, Valid: true},
		}); err != nil {
			return fmt.Errorf("clear for invoice: %w", err)
		}
		return nil
	})
}

// Delete removes a shift by uuid and writes one audit row, atomically. The audit
// EntityID keeps the int PK, resolved in-tx; a missing row is a no-op.
func (r *ShiftsRepo) Delete(ctx context.Context, tenantID int64, shiftUUID string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		id, e := q.GetShiftIDByUUID(ctx, gen.GetShiftIDByUUIDParams{TenantID: tenantID, Uuid: shiftUUID})
		if errors.Is(e, sql.ErrNoRows) {
			return nil // missing row → silent no-op
		}
		if e != nil {
			return fmt.Errorf("resolve shift: %w", e)
		}
		if err := q.DeleteShift(ctx, gen.DeleteShiftParams{TenantID: tenantID, Uuid: shiftUUID}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{EntityType: "shift", EntityID: id, Action: "delete"})
	})
}

// ListItems returns a shift's line items (billed and unbilled), oldest first.
func (r *ShiftsRepo) ListItems(ctx context.Context, tenantID, shiftID int64) ([]*billing.LineItem, error) {
	if tenantID == 0 || shiftID == 0 {
		return nil, errors.New("list shift items: tenant and shift id required")
	}
	rows, err := gen.New(r.db).ListLineItemsForShift(ctx, gen.ListLineItemsForShiftParams{
		TenantID: tenantID, ShiftID: sql.NullInt64{Int64: shiftID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list shift items: %w", err)
	}
	out := make([]*billing.LineItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, billing.LineItemFromRow(billing.LineItemRowFromShiftList(rows[i])))
	}
	return out, nil
}

// GetItem returns one line item by id, or (nil, nil) when absent for the tenant.
func (r *ShiftsRepo) GetItem(ctx context.Context, tenantID, itemID int64) (*billing.LineItem, error) {
	if tenantID == 0 || itemID == 0 {
		return nil, errors.New("get shift item: tenant and item id required")
	}
	row, err := gen.New(r.db).GetLineItem(ctx, gen.GetLineItemParams{TenantID: tenantID, ID: itemID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shift item: %w", err)
	}
	return billing.LineItemFromRow(billing.LineItemRowFromGet(row)), nil
}

// CountItems returns how many UNBILLED items the shift carries.
func (r *ShiftsRepo) CountItems(ctx context.Context, tenantID, shiftID int64) (int64, error) {
	if tenantID == 0 || shiftID == 0 {
		return 0, errors.New("count shift items: tenant and shift id required")
	}
	n, err := gen.New(r.db).CountShiftItems(ctx, gen.CountShiftItemsParams{
		TenantID: tenantID, ShiftID: sql.NullInt64{Int64: shiftID, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("count shift items: %w", err)
	}
	return n, nil
}

// CreateItem inserts a line item on a shift (shift_id set, invoice_id NULL) and
// writes one audit row. in is expected pre-priced by the caller.
func (r *ShiftsRepo) CreateItem(ctx context.Context, tenantID, shiftID int64, in billing.LineItemInput) (*billing.LineItem, error) {
	if tenantID == 0 || shiftID == 0 {
		return nil, errors.New("create shift item: tenant and shift id required")
	}
	if in.Quantity < 0 {
		return nil, errors.New("create shift item: quantity must not be negative")
	}
	var newID int64
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: shiftID, Action: "create",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		customItemID, e := billing.ResolveCustomItemID(ctx, q, tenantID, in.CustomItemID)
		if e != nil {
			return e
		}
		row, e := q.CreateLineItem(ctx, lineItemParams(tenantID, &shiftID, customItemID, in))
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newID = row.ID
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create shift item: %w", err)
	}
	return r.GetItem(ctx, tenantID, newID)
}

// UpdateItem rewrites an UNBILLED shift item (invoice_id IS NULL guard) and
// writes one audit row. Returns (nil, nil) when the item is absent or already
// billed. in is expected pre-priced by the caller.
func (r *ShiftsRepo) UpdateItem(ctx context.Context, tenantID, itemID int64, in billing.LineItemInput) (*billing.LineItem, error) {
	if tenantID == 0 || itemID == 0 {
		return nil, errors.New("update shift item: tenant and item id required")
	}
	if in.Quantity < 0 {
		return nil, errors.New("update shift item: quantity must not be negative")
	}
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: itemID, Action: "update",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		customItemID, e := billing.ResolveCustomItemID(ctx, q, tenantID, in.CustomItemID)
		if e != nil {
			return e
		}
		_, e = q.UpdateShiftLineItem(ctx, gen.UpdateShiftLineItemParams{
			SupportItemID:    db.NullStr(in.SupportItemID),
			CustomItemID:     customItemID,
			CatalogVersionID: db.NullStr(in.CatalogVersionID),
			Code:             db.NzMaybe(in.Code),
			Description:      in.Description,
			ServiceDate:      db.NzMaybe(in.ServiceDate),
			Unit:             db.NzMaybe(in.Unit),
			StartTime:        db.NzMaybe(in.StartTime),
			EndTime:          db.NzMaybe(in.EndTime),
			Quantity:         in.Quantity,
			UnitPrice:        in.UnitPrice,
			Taxable:          db.B2i(in.Taxable),
			LineTotal:        billing.Round2(in.Quantity * in.UnitPrice),
			TenantID:         tenantID,
			ID:               itemID,
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
		return nil, fmt.Errorf("update shift item: %w", err)
	}
	return r.GetItem(ctx, tenantID, itemID)
}

// DeleteUnbilledItems removes ALL of a shift's unbilled items (invoice_id IS
// NULL) in one audited mutation. Used to make a re-divide idempotent.
func (r *ShiftsRepo) DeleteUnbilledItems(ctx context.Context, tenantID, shiftID int64) error {
	if tenantID == 0 || shiftID == 0 {
		return errors.New("delete unbilled items: tenant and shift id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: shiftID, Action: "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteUnbilledItemsForShift(ctx, gen.DeleteUnbilledItemsForShiftParams{
			TenantID: tenantID, ShiftID: sql.NullInt64{Int64: shiftID, Valid: true},
		}); err != nil {
			return fmt.Errorf("delete unbilled items: %w", err)
		}
		return nil
	})
}

// DeleteItem removes an UNBILLED shift item (invoice_id IS NULL guard) and writes
// one audit row.
func (r *ShiftsRepo) DeleteItem(ctx context.Context, tenantID, itemID int64) error {
	if tenantID == 0 || itemID == 0 {
		return errors.New("delete shift item: tenant and item id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: itemID, Action: "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteShiftLineItem(ctx, gen.DeleteShiftLineItemParams{TenantID: tenantID, ID: itemID}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// GetItemByUUID returns a shift's line item addressed by uuid, scoped to the
// owning shift's int id, or (nil, nil) when absent. The shift scope ensures an
// item uuid from another shift (or tenant) 404s.
func (r *ShiftsRepo) GetItemByUUID(ctx context.Context, tenantID, shiftID int64, itemUUID string) (*billing.LineItem, error) {
	if tenantID == 0 || shiftID == 0 {
		return nil, errors.New("get shift item: tenant and shift id required")
	}
	row, err := gen.New(r.db).GetShiftLineItemByUUID(ctx, gen.GetShiftLineItemByUUIDParams{
		TenantID: tenantID, ShiftID: sql.NullInt64{Int64: shiftID, Valid: true}, Uuid: itemUUID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shift item by uuid: %w", err)
	}
	return billing.LineItemFromRow(billing.LineItemRowFromShiftUUID(row)), nil
}

// UpdateItemByUUID rewrites an UNBILLED shift item addressed by uuid (scoped to
// the owning shift, invoice_id IS NULL guard) and writes one audit row. Returns
// (nil, nil) when the item is absent or already billed. in is expected
// pre-priced by the caller. The audit EntityID keeps the item's int PK.
func (r *ShiftsRepo) UpdateItemByUUID(ctx context.Context, tenantID, shiftID int64, itemUUID string, in billing.LineItemInput) (*billing.LineItem, error) {
	if tenantID == 0 || shiftID == 0 {
		return nil, errors.New("update shift item: tenant and shift id required")
	}
	if in.Quantity < 0 {
		return nil, errors.New("update shift item: quantity must not be negative")
	}
	var item *billing.LineItem
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		customItemID, e := billing.ResolveCustomItemID(ctx, q, tenantID, in.CustomItemID)
		if e != nil {
			return e
		}
		row, e := q.UpdateShiftLineItemByUUID(ctx, gen.UpdateShiftLineItemByUUIDParams{
			SupportItemID:    db.NullStr(in.SupportItemID),
			CustomItemID:     customItemID,
			CatalogVersionID: db.NullStr(in.CatalogVersionID),
			Code:             db.NzMaybe(in.Code),
			Description:      in.Description,
			ServiceDate:      db.NzMaybe(in.ServiceDate),
			Unit:             db.NzMaybe(in.Unit),
			StartTime:        db.NzMaybe(in.StartTime),
			EndTime:          db.NzMaybe(in.EndTime),
			Quantity:         in.Quantity,
			UnitPrice:        in.UnitPrice,
			Taxable:          db.B2i(in.Taxable),
			LineTotal:        billing.Round2(in.Quantity * in.UnitPrice),
			TenantID:         tenantID,
			ShiftID:          sql.NullInt64{Int64: shiftID, Valid: true},
			Uuid:             itemUUID,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		item = billing.LineItemFromRow(lineItemRowFromGen(row, in.CustomItemID))
		return audit.Log(ctx, tx, audit.Entry{EntityType: "line_item", EntityID: row.ID, Action: "update"})
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update shift item by uuid: %w", err)
	}
	return item, nil
}

// DeleteItemByUUID removes an UNBILLED shift item addressed by uuid (scoped to
// the owning shift, invoice_id IS NULL guard) and writes one audit row. A
// missing/billed item is a no-op. The audit EntityID keeps the item's int PK,
// resolved in-tx.
func (r *ShiftsRepo) DeleteItemByUUID(ctx context.Context, tenantID, shiftID int64, itemUUID string) error {
	if tenantID == 0 || shiftID == 0 {
		return errors.New("delete shift item: tenant and shift id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetShiftLineItemByUUID(ctx, gen.GetShiftLineItemByUUIDParams{
			TenantID: tenantID, ShiftID: sql.NullInt64{Int64: shiftID, Valid: true}, Uuid: itemUUID,
		})
		if errors.Is(e, sql.ErrNoRows) {
			return nil // missing → no-op
		}
		if e != nil {
			return fmt.Errorf("resolve item: %w", e)
		}
		if err := q.DeleteShiftLineItemByUUID(ctx, gen.DeleteShiftLineItemByUUIDParams{
			TenantID: tenantID, ShiftID: sql.NullInt64{Int64: shiftID, Valid: true}, Uuid: itemUUID,
		}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{EntityType: "line_item", EntityID: row.ID, Action: "delete"})
	})
}

// lineItemRowFromGen adapts a bare gen.LineItem (an UPDATE/INSERT RETURNING *
// row, which carries no custom-item join) into a billing.LineItemRow, stamping
// the custom-item uuid from the resolved inbound value so customItemId
// round-trips without a re-read.
func lineItemRowFromGen(r gen.LineItem, customItemUUID *string) billing.LineItemRow {
	return billing.LineItemRow{
		ID: r.ID, Uuid: r.Uuid, ShiftID: r.ShiftID, InvoiceID: r.InvoiceID,
		SupportItemID: r.SupportItemID, CustomItemID: r.CustomItemID, CustomItemUuid: db.NullStr(customItemUUID),
		CatalogVersionID: r.CatalogVersionID, Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

// lineItemParams builds the gen insert params for a line item. shiftID nil = an
// invoice-only line; here it is always set (shift item, invoice_id NULL). The
// inbound custom-item uuid is resolved to the int FK by the caller and passed in.
func lineItemParams(tenantID int64, shiftID *int64, customItemID sql.NullInt64, in billing.LineItemInput) gen.CreateLineItemParams {
	return gen.CreateLineItemParams{
		Uuid:             uuid.NewString(),
		TenantID:         tenantID,
		ShiftID:          db.NullID(shiftID),
		InvoiceID:        sql.NullInt64{}, // unbilled shift item
		SupportItemID:    db.NullStr(in.SupportItemID),
		CustomItemID:     customItemID,
		CatalogVersionID: db.NullStr(in.CatalogVersionID),
		Code:             db.NzMaybe(in.Code),
		Description:      in.Description,
		ServiceDate:      db.NzMaybe(in.ServiceDate),
		Unit:             db.NzMaybe(in.Unit),
		StartTime:        db.NzMaybe(in.StartTime),
		EndTime:          db.NzMaybe(in.EndTime),
		Quantity:         in.Quantity,
		UnitPrice:        in.UnitPrice,
		Taxable:          db.B2i(in.Taxable),
		LineTotal:        billing.Round2(in.Quantity * in.UnitPrice),
		SortOrder:        sql.NullInt64{Int64: in.SortOrder, Valid: true},
	}
}

// UnbilledByClient aggregates the tenant's recorded-but-unbilled shifts per
// client (count and service-date span), ready for billing suggestions.
func (r *ShiftsRepo) UnbilledByClient(ctx context.Context, tenantID int64) ([]UnbilledAgg, error) {
	if tenantID == 0 {
		return nil, errors.New("unbilled by client: tenant id required")
	}
	rows, err := gen.New(r.db).ClientUnbilledAgg(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("unbilled by client: %w", err)
	}
	out := make([]UnbilledAgg, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, UnbilledAgg{
			ClientID: rows[i].ClientID,
			Count:    rows[i].Cnt,
			From:     anyToString(rows[i].FromDate),
			To:       anyToString(rows[i].ToDate),
		})
	}
	return out, nil
}

// encodeTags marshals tags to JSON TEXT, defaulting a nil slice to an empty
// array so the column is never NULL/"null".
func encodeTags(tags []string) (string, error) {
	if tags == nil {
		tags = []string{}
	}
	tb, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("shift: marshal tags: %w", err)
	}
	return string(tb), nil
}

// anyToString coerces a SQLite MIN/MAX aggregate (scanned as interface{}) into a
// string. Empty groups would not appear (GROUP BY), so a nil yields "".
func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// shiftFields is the common projection shared by every enriched shift read row
// (Get/GetByID/List*). The gen row types are nominally distinct but identical in
// shape; each is adapted into this struct so a single mapper (mapShift) builds
// the DTO. ClientUUID is the joined clients.uuid (NULL if the FK is
// dangling, which the clean schema forbids).
type shiftFields struct {
	id           int64
	uuid         string
	clientID     int64
	clientUUID   sql.NullString
	serviceDate  string
	note         string
	tags         string
	status       string
	invoiceID    sql.NullInt64
	invoiceUUID  sql.NullString
	authorUserID sql.NullInt64
	createdAt    string
	updatedAt    string
}

func mapShift(f shiftFields) (*Shift, error) {
	tags := []string{}
	if f.tags != "" {
		if err := json.Unmarshal([]byte(f.tags), &tags); err != nil {
			return nil, fmt.Errorf("shift %d: unmarshal tags: %w", f.id, err)
		}
		if tags == nil {
			tags = []string{}
		}
	}
	return &Shift{
		ID:           f.id,
		UUID:         f.uuid,
		ClientID:     f.clientID,
		ClientUUID:   f.clientUUID.String,
		ServiceDate:  f.serviceDate,
		Note:         f.note,
		Tags:         tags,
		Status:       f.status,
		InvoiceID:    db.PtrID(f.invoiceID),
		InvoiceUUID:  db.PtrStr(f.invoiceUUID),
		AuthorUserID: db.PtrID(f.authorUserID),
		CreatedAt:    f.createdAt,
		UpdatedAt:    f.updatedAt,
	}, nil
}

// shiftFieldsFromGet / *FromByID / *FromList* adapt the (nominally distinct but
// identically shaped) enriched gen row types into the common shiftFields. They
// keep the per-query row scanners while a single mapper builds the DTO.
func shiftFieldsFromGet(r gen.GetShiftRow) shiftFields {
	return shiftFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func shiftFieldsFromByID(r gen.GetShiftByIDRow) shiftFields {
	return shiftFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func shiftFieldsFromList(r gen.ListShiftsRow) shiftFields {
	return shiftFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func shiftFieldsFromByPart(r gen.ListShiftsByClientRow) shiftFields {
	return shiftFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func shiftFieldsFromByPartRange(r gen.ListShiftsByClientRangeRow) shiftFields {
	return shiftFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func shiftFieldsFromByStatus(r gen.ListShiftsByStatusRow) shiftFields {
	return shiftFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func shiftFieldsFromScheduled(r gen.ListScheduledShiftsRow) shiftFields {
	return shiftFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func shiftFieldsFromRecorded(r gen.ListRecordedUnbilledByClientRow) shiftFields {
	return shiftFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

// mapShifts maps a slice of enriched gen rows (via an adapter to shiftFields)
// into DTOs. Bounded by len(rows).
func mapShifts[T any](rows []T, adapt func(T) shiftFields) ([]*Shift, error) {
	out := make([]*Shift, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		s, err := mapShift(adapt(rows[i]))
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// validISODate reports whether s is a strict YYYY-MM-DD calendar date.
func validISODate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
