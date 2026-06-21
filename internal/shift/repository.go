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
// unit a provider records for a participant. A shift's billable quantities live
// on its line_items rows (see ListItems), not on the shift itself. Tags is
// stored as JSON TEXT and is never nil. Status moves through the lifecycle
// scheduled→recorded→drafted→sent→paid; InvoiceID is set once the shift is
// drafted onto an invoice.
type Shift struct {
	ID            int64    `json:"id"`
	UUID          string   `json:"uuid"`
	ParticipantID int64    `json:"participantId"`
	ServiceDate   string   `json:"serviceDate"`
	Note          string   `json:"note"`
	Tags          []string `json:"tags"`
	Status        string   `json:"status"`
	InvoiceID     *int64   `json:"invoiceId"`
	AuthorUserID  *int64   `json:"authorUserId"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

// ShiftInput is the writable subset of a shift.
type ShiftInput struct {
	ParticipantID int64    `json:"participantId"`
	ServiceDate   string   `json:"serviceDate"`
	Note          string   `json:"note"`
	Tags          []string `json:"tags"`
	Status        string   `json:"status"`
}

// UnbilledAgg summarises a participant's recorded-but-unbilled shifts: how many there
// are and the service-date span they cover.
type UnbilledAgg struct {
	ParticipantID int64  `json:"participantId"`
	Count         int64  `json:"count"`
	From          string `json:"from"`
	To            string `json:"to"`
}

// ShiftsRepo reads and writes the shifts table (tenant-scoped) with audited
// mutations.
type ShiftsRepo struct {
	db *sql.DB
}

// NewShifts constructs a repository. A nil db is a programmer error.
func NewShifts(db *sql.DB) *ShiftsRepo {
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
	if in.ParticipantID == 0 {
		return nil, errors.New("create shift: participant id required")
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
			Uuid:          uuid.NewString(),
			TenantID:      tenantID,
			ParticipantID: in.ParticipantID,
			ServiceDate:   in.ServiceDate,
			Note:          in.Note,
			Tags:          tags,
			Status:        status,
			AuthorUserID:  db.NullID(authorUserID),
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newID = s.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "shift", EntityID: s.ID, Action: "create",
			Changes: audit.Changes(map[string]any{"participantId": in.ParticipantID, "serviceDate": in.ServiceDate}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create shift: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// Get returns the tenant's shift by id, or (nil, nil) when absent.
func (r *ShiftsRepo) Get(ctx context.Context, tenantID, id int64) (*Shift, error) {
	row, err := gen.New(r.db).GetShift(ctx, gen.GetShiftParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shift: %w", err)
	}
	return toShift(row)
}

// ListParticipant returns a participant's shifts. When both from and to are
// non-empty it restricts to service_date ∈ [from, to]; otherwise it returns all.
func (r *ShiftsRepo) ListParticipant(ctx context.Context, tenantID, participantID int64, from, to string) ([]*Shift, error) {
	if tenantID == 0 || participantID == 0 {
		return nil, errors.New("list shifts: tenant and participant id required")
	}
	q := gen.New(r.db)
	if from != "" && to != "" {
		rows, err := q.ListShiftsByParticipantRange(ctx, gen.ListShiftsByParticipantRangeParams{
			TenantID: tenantID, ParticipantID: participantID, ServiceDate: from, ServiceDate_2: to,
		})
		if err != nil {
			return nil, fmt.Errorf("list participant shifts range: %w", err)
		}
		return toShifts(rows)
	}
	rows, err := q.ListShiftsByParticipant(ctx, gen.ListShiftsByParticipantParams{
		TenantID: tenantID, ParticipantID: participantID,
	})
	if err != nil {
		return nil, fmt.Errorf("list participant shifts: %w", err)
	}
	return toShifts(rows)
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
	return toShifts(rows)
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
	return toShifts(rows)
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
	return toShifts(rows)
}

// ListRecordedUnbilled returns a participant's recorded shifts that are not yet
// linked to an invoice (status 'recorded', invoice_id NULL).
func (r *ShiftsRepo) ListRecordedUnbilled(ctx context.Context, tenantID, participantID int64) ([]*Shift, error) {
	if tenantID == 0 || participantID == 0 {
		return nil, errors.New("list recorded unbilled: tenant and participant id required")
	}
	rows, err := gen.New(r.db).ListRecordedUnbilledByParticipant(ctx, gen.ListRecordedUnbilledByParticipantParams{
		TenantID: tenantID, ParticipantID: participantID,
	})
	if err != nil {
		return nil, fmt.Errorf("list recorded unbilled: %w", err)
	}
	return toShifts(rows)
}

// Update rewrites a shift's editable fields and writes one audit row, atomically.
// Returns (nil, nil) when the shift does not exist for the tenant.
func (r *ShiftsRepo) Update(ctx context.Context, tenantID, id int64, in ShiftInput) (*Shift, error) {
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
	err = audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: id, Action: "update",
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, e := gen.New(tx).UpdateShift(ctx, gen.UpdateShiftParams{
			ServiceDate: in.ServiceDate,
			Note:        in.Note,
			Tags:        tags,
			Status:      status,
			UpdatedAt:   now,
			TenantID:    tenantID,
			ID:          id,
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
		return nil, fmt.Errorf("update shift: %w", err)
	}
	return r.Get(ctx, tenantID, id)
}

// UpdateStatus sets a shift's lifecycle status and writes one audit row.
func (r *ShiftsRepo) UpdateStatus(ctx context.Context, tenantID, id int64, status string) error {
	if status == "" {
		return errors.New("update shift status: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).UpdateShiftStatus(ctx, gen.UpdateShiftStatusParams{
			Status: status, UpdatedAt: now, TenantID: tenantID, ID: id,
		}); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		return nil
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

// Delete removes a shift and writes one audit row, atomically.
func (r *ShiftsRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteShift(ctx, gen.DeleteShiftParams{TenantID: tenantID, ID: id}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
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
		out = append(out, billing.LineItemFromRow(rows[i]))
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
	return billing.LineItemFromRow(row), nil
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
		row, e := gen.New(tx).CreateLineItem(ctx, lineItemParams(tenantID, &shiftID, in))
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
		_, e := gen.New(tx).UpdateShiftLineItem(ctx, gen.UpdateShiftLineItemParams{
			SupportItemID:    db.NullStr(in.SupportItemID),
			CustomItemID:     db.NullID(in.CustomItemID),
			CatalogVersionID: db.NullStr(in.CatalogVersionID),
			Code:             db.NzMaybe(in.Code),
			Description:      in.Description,
			ServiceDate:      db.NzMaybe(in.ServiceDate),
			Unit:             db.NzMaybe(in.Unit),
			StartTime:        db.NzMaybe(in.StartTime),
			EndTime:          db.NzMaybe(in.EndTime),
			Quantity:         in.Quantity,
			UnitPrice:        in.UnitPrice,
			GstFree:          db.B2i(in.GstFree),
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

// lineItemParams builds the gen insert params for a line item. shiftID nil = an
// invoice-only line; here it is always set (shift item, invoice_id NULL).
func lineItemParams(tenantID int64, shiftID *int64, in billing.LineItemInput) gen.CreateLineItemParams {
	return gen.CreateLineItemParams{
		Uuid:             uuid.NewString(),
		TenantID:         tenantID,
		ShiftID:          db.NullID(shiftID),
		InvoiceID:        sql.NullInt64{}, // unbilled shift item
		SupportItemID:    db.NullStr(in.SupportItemID),
		CustomItemID:     db.NullID(in.CustomItemID),
		CatalogVersionID: db.NullStr(in.CatalogVersionID),
		Code:             db.NzMaybe(in.Code),
		Description:      in.Description,
		ServiceDate:      db.NzMaybe(in.ServiceDate),
		Unit:             db.NzMaybe(in.Unit),
		StartTime:        db.NzMaybe(in.StartTime),
		EndTime:          db.NzMaybe(in.EndTime),
		Quantity:         in.Quantity,
		UnitPrice:        in.UnitPrice,
		GstFree:          db.B2i(in.GstFree),
		LineTotal:        billing.Round2(in.Quantity * in.UnitPrice),
		SortOrder:        sql.NullInt64{Int64: in.SortOrder, Valid: true},
	}
}

// UnbilledByParticipant aggregates the tenant's recorded-but-unbilled shifts per
// participant (count and service-date span), ready for billing suggestions.
func (r *ShiftsRepo) UnbilledByParticipant(ctx context.Context, tenantID int64) ([]UnbilledAgg, error) {
	if tenantID == 0 {
		return nil, errors.New("unbilled by participant: tenant id required")
	}
	rows, err := gen.New(r.db).ParticipantUnbilledAgg(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("unbilled by participant: %w", err)
	}
	out := make([]UnbilledAgg, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, UnbilledAgg{
			ParticipantID: rows[i].ParticipantID,
			Count:         rows[i].Cnt,
			From:          anyToString(rows[i].FromDate),
			To:            anyToString(rows[i].ToDate),
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

func toShift(r gen.Shift) (*Shift, error) {
	tags := []string{}
	if r.Tags != "" {
		if err := json.Unmarshal([]byte(r.Tags), &tags); err != nil {
			return nil, fmt.Errorf("shift %d: unmarshal tags: %w", r.ID, err)
		}
		if tags == nil {
			tags = []string{}
		}
	}
	return &Shift{
		ID:            r.ID,
		UUID:          r.Uuid,
		ParticipantID: r.ParticipantID,
		ServiceDate:   r.ServiceDate,
		Note:          r.Note,
		Tags:          tags,
		Status:        r.Status,
		InvoiceID:     db.PtrID(r.InvoiceID),
		AuthorUserID:  db.PtrID(r.AuthorUserID),
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}, nil
}

func toShifts(rows []gen.Shift) ([]*Shift, error) {
	out := make([]*Shift, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		s, err := toShift(rows[i])
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
