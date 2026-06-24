// Package session is the session vertical slice: domain types, the audited
// repository over the sessions table, the service (with SSE broadcast), and the
// HTTP handler. It depends only on platform packages (db/gen, audit, reqctx,
// realtime, httpx), never on other domain slices.
package session

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
	"github.com/dknathalage/tallyo/internal/ids"
)

// Session is the domain view of a row in the sessions table — the delivered-support
// unit a provider records for a client. A session's billable quantities live
// on its line_items rows (see ListItems), not on the session itself. Tags is
// stored as JSON TEXT and is never nil. Status moves through the lifecycle
// scheduled→recorded→drafted→sent→paid; InvoiceID is set once the session is
// drafted onto an invoice.
type Session struct {
	ID           int64    `json:"-"`
	UUID         string   `json:"id"`
	ClientID     int64    `json:"-"`
	ClientUUID   string   `json:"clientId"`
	ServiceDate  string   `json:"serviceDate"`
	Note         string   `json:"note"`
	Tags         []string `json:"tags"`
	Status       string   `json:"status"`
	InvoiceID    *int64   `json:"-"`         // internal FK; the public ref is invoiceId (the linked invoice's uuid)
	InvoiceUUID  *string  `json:"invoiceId"` // linked invoice uuid (nil until the session is drafted onto an invoice)
	AuthorUserID *int64   `json:"-"`         // internal author user FK; not linked from the SPA
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// SessionInput is the writable subset of a session.
type SessionInput struct {
	ClientID    int64    `json:"clientId"`
	ServiceDate string   `json:"serviceDate"`
	Note        string   `json:"note"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
}

// UnbilledAgg summarises a client's recorded-but-unbilled sessions: how many there
// are and the service-date span they cover.
type UnbilledAgg struct {
	ClientID int64  `json:"clientId"`
	Count    int64  `json:"count"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// SessionsRepo reads and writes the sessions table (tenant-scoped) with audited
// mutations.
type SessionsRepo struct {
	db db.Executor
}

// NewSessions constructs a repository. A nil db is a programmer error.
func NewSessions(db db.Executor) *SessionsRepo {
	if db == nil {
		panic("session: NewSessions requires a non-nil *sql.DB")
	}
	return &SessionsRepo{db: db}
}

// Create inserts a session and writes one audit row, atomically. authorUserID is
// the user the session is attributed to (nil when unknown). Status defaults to
// 'recorded' when the input leaves it empty.
func (r *SessionsRepo) Create(ctx context.Context, tenantID int64, authorUserID *int64, in SessionInput) (*Session, error) {
	if tenantID == 0 {
		return nil, errors.New("create session: tenant id required")
	}
	if in.ClientID == 0 {
		return nil, errors.New("create session: client id required")
	}
	if !validISODate(in.ServiceDate) {
		return nil, errors.New("create session: service date must be a valid YYYY-MM-DD date")
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
		s, e := gen.New(tx).CreateSession(ctx, gen.CreateSessionParams{
			Uuid:         ids.New(),
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
			EntityType: "session", EntityID: s.ID, Action: "create",
			Changes: audit.Changes(map[string]any{"clientId": in.ClientID, "serviceDate": in.ServiceDate}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// Get returns the tenant's session by int PK, or (nil, nil) when absent. This is
// the internal/cross-slice read (agent SessionReader, the service's own pricing
// path); the public HTTP path addresses sessions by uuid via GetByUUID.
func (r *SessionsRepo) Get(ctx context.Context, tenantID, id int64) (*Session, error) {
	row, err := gen.New(r.db).GetSessionByID(ctx, gen.GetSessionByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return mapSession(sessionFieldsFromByID(row))
}

// GetByUUID returns the tenant's session by uuid, or (nil, nil) when absent (or
// owned by another tenant — the query is tenant-scoped). This is the public
// HTTP read.
func (r *SessionsRepo) GetByUUID(ctx context.Context, tenantID int64, sessionUUID string) (*Session, error) {
	row, err := gen.New(r.db).GetSession(ctx, gen.GetSessionParams{TenantID: tenantID, Uuid: sessionUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session by uuid: %w", err)
	}
	return mapSession(sessionFieldsFromGet(row))
}

// ResolveID translates a session uuid into its int PK for the tenant. Returns
// (0, nil) when no such session exists (so callers can 404 without an error).
func (r *SessionsRepo) ResolveID(ctx context.Context, tenantID int64, sessionUUID string) (int64, error) {
	id, err := gen.New(r.db).GetSessionIDByUUID(ctx, gen.GetSessionIDByUUIDParams{TenantID: tenantID, Uuid: sessionUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve session uuid: %w", err)
	}
	return id, nil
}

// ResolveClientID translates a client uuid into its int PK for the
// tenant (used by the ?client= session filter and inbound clientId
// resolution). Returns (0, nil) when absent.
func (r *SessionsRepo) ResolveClientID(ctx context.Context, tenantID int64, clientUUID string) (int64, error) {
	id, err := gen.New(r.db).GetClientIDByUUID(ctx, gen.GetClientIDByUUIDParams{TenantID: tenantID, Uuid: clientUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve client uuid: %w", err)
	}
	return id, nil
}

// ListClient returns a client's sessions. When both from and to are
// non-empty it restricts to service_date ∈ [from, to]; otherwise it returns all.
func (r *SessionsRepo) ListClient(ctx context.Context, tenantID, clientID int64, from, to string) ([]*Session, error) {
	if tenantID == 0 || clientID == 0 {
		return nil, errors.New("list sessions: tenant and client id required")
	}
	q := gen.New(r.db)
	if from != "" && to != "" {
		rows, err := q.ListSessionsByClientRange(ctx, gen.ListSessionsByClientRangeParams{
			TenantID: tenantID, ClientID: clientID, ServiceDate: from, ServiceDate_2: to,
		})
		if err != nil {
			return nil, fmt.Errorf("list client sessions range: %w", err)
		}
		return mapSessions(rows, sessionFieldsFromByPartRange)
	}
	rows, err := q.ListSessionsByClient(ctx, gen.ListSessionsByClientParams{
		TenantID: tenantID, ClientID: clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("list client sessions: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromByPart)
}

// List returns all of the tenant's sessions (newest service date first).
func (r *SessionsRepo) List(ctx context.Context, tenantID int64) ([]*Session, error) {
	if tenantID == 0 {
		return nil, errors.New("list sessions: tenant id required")
	}
	rows, err := gen.New(r.db).ListSessions(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromList)
}

// ListByStatus returns the tenant's sessions in a given lifecycle status.
func (r *SessionsRepo) ListByStatus(ctx context.Context, tenantID int64, status string) ([]*Session, error) {
	if tenantID == 0 {
		return nil, errors.New("list sessions by status: tenant id required")
	}
	rows, err := gen.New(r.db).ListSessionsByStatus(ctx, gen.ListSessionsByStatusParams{TenantID: tenantID, Status: status})
	if err != nil {
		return nil, fmt.Errorf("list sessions by status: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromByStatus)
}

// ListScheduled returns the tenant's scheduled (not yet recorded) sessions.
func (r *SessionsRepo) ListScheduled(ctx context.Context, tenantID int64) ([]*Session, error) {
	if tenantID == 0 {
		return nil, errors.New("list scheduled sessions: tenant id required")
	}
	rows, err := gen.New(r.db).ListScheduledSessions(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list scheduled sessions: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromScheduled)
}

// ListRecordedUnbilled returns a client's recorded sessions that are not yet
// linked to an invoice (status 'recorded', invoice_id NULL).
func (r *SessionsRepo) ListRecordedUnbilled(ctx context.Context, tenantID, clientID int64) ([]*Session, error) {
	if tenantID == 0 || clientID == 0 {
		return nil, errors.New("list recorded unbilled: tenant and client id required")
	}
	rows, err := gen.New(r.db).ListRecordedUnbilledByClient(ctx, gen.ListRecordedUnbilledByClientParams{
		TenantID: tenantID, ClientID: clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("list recorded unbilled: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromRecorded)
}

// Update rewrites a session's editable fields by uuid and writes one audit row,
// atomically. Returns (nil, nil) when the session does not exist for the tenant.
// The audit EntityID keeps the int PK, recovered from the RETURNING row.
func (r *SessionsRepo) Update(ctx context.Context, tenantID int64, sessionUUID string, in SessionInput) (*Session, error) {
	if !validISODate(in.ServiceDate) {
		return nil, errors.New("update session: service date must be a valid YYYY-MM-DD date")
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
		row, e := gen.New(tx).UpdateSession(ctx, gen.UpdateSessionParams{
			ServiceDate: in.ServiceDate,
			Note:        in.Note,
			Tags:        tags,
			Status:      status,
			UpdatedAt:   now,
			TenantID:    tenantID,
			Uuid:        sessionUUID,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{EntityType: "session", EntityID: row.ID, Action: "update"})
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update session: %w", err)
	}
	return r.GetByUUID(ctx, tenantID, sessionUUID)
}

// UpdateStatus sets a session's lifecycle status by uuid and writes one audit row.
// The audit EntityID keeps the int PK, resolved in-tx; a missing row is a no-op.
func (r *SessionsRepo) UpdateStatus(ctx context.Context, tenantID int64, sessionUUID, status string) error {
	if status == "" {
		return errors.New("update session status: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		id, e := q.GetSessionIDByUUID(ctx, gen.GetSessionIDByUUIDParams{TenantID: tenantID, Uuid: sessionUUID})
		if errors.Is(e, sql.ErrNoRows) {
			return nil // missing row → silent no-op
		}
		if e != nil {
			return fmt.Errorf("resolve session: %w", e)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if err := q.UpdateSessionStatus(ctx, gen.UpdateSessionStatusParams{
			Status: status, UpdatedAt: now, TenantID: tenantID, Uuid: sessionUUID,
		}); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "session", EntityID: id, Action: "status",
			Changes: audit.Changes(map[string]any{"status": status}),
		})
	})
}

// SetInvoice links a session to an invoice and sets its status, atomically.
func (r *SessionsRepo) SetInvoice(ctx context.Context, tenantID, id, invoiceID int64, status string) error {
	if invoiceID == 0 {
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
			InvoiceID: sql.NullInt64{Int64: invoiceID, Valid: true}, Status: status,
			UpdatedAt: now, TenantID: tenantID, ID: id,
		}); err != nil {
			return fmt.Errorf("set invoice: %w", err)
		}
		return nil
	})
}

// SetStatusForInvoice sets the status of every session linked to an invoice (e.g.
// cascading 'sent'/'paid' from the invoice), atomically.
func (r *SessionsRepo) SetStatusForInvoice(ctx context.Context, tenantID, invoiceID int64, status string) error {
	if invoiceID == 0 {
		return errors.New("set status for invoice: invoice id required")
	}
	if status == "" {
		return errors.New("set status for invoice: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "session", EntityID: 0, Action: "status",
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

// ClearForInvoice reverts every session linked to an invoice back to 'recorded'
// with a NULL invoice_id (used when the invoice is deleted), atomically.
func (r *SessionsRepo) ClearForInvoice(ctx context.Context, tenantID, invoiceID int64) error {
	if invoiceID == 0 {
		return errors.New("clear sessions for invoice: invoice id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "session", EntityID: 0, Action: "unbill",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).ClearSessionsForInvoice(ctx, gen.ClearSessionsForInvoiceParams{
			UpdatedAt: now, TenantID: tenantID,
			InvoiceID: sql.NullInt64{Int64: invoiceID, Valid: true},
		}); err != nil {
			return fmt.Errorf("clear for invoice: %w", err)
		}
		return nil
	})
}

// Delete removes a session by uuid and writes one audit row, atomically. The audit
// EntityID keeps the int PK, resolved in-tx; a missing row is a no-op.
func (r *SessionsRepo) Delete(ctx context.Context, tenantID int64, sessionUUID string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		id, e := q.GetSessionIDByUUID(ctx, gen.GetSessionIDByUUIDParams{TenantID: tenantID, Uuid: sessionUUID})
		if errors.Is(e, sql.ErrNoRows) {
			return nil // missing row → silent no-op
		}
		if e != nil {
			return fmt.Errorf("resolve session: %w", e)
		}
		if err := q.DeleteSession(ctx, gen.DeleteSessionParams{TenantID: tenantID, Uuid: sessionUUID}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{EntityType: "session", EntityID: id, Action: "delete"})
	})
}

// ListItems returns a session's line items (billed and unbilled), oldest first.
func (r *SessionsRepo) ListItems(ctx context.Context, tenantID, sessionID int64) ([]*billing.LineItem, error) {
	if tenantID == 0 || sessionID == 0 {
		return nil, errors.New("list session items: tenant and session id required")
	}
	rows, err := gen.New(r.db).ListLineItemsForSession(ctx, gen.ListLineItemsForSessionParams{
		TenantID: tenantID, SessionID: sql.NullInt64{Int64: sessionID, Valid: true},
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
func (r *SessionsRepo) GetItem(ctx context.Context, tenantID, itemID int64) (*billing.LineItem, error) {
	if tenantID == 0 || itemID == 0 {
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
func (r *SessionsRepo) CountItems(ctx context.Context, tenantID, sessionID int64) (int64, error) {
	if tenantID == 0 || sessionID == 0 {
		return 0, errors.New("count session items: tenant and session id required")
	}
	n, err := gen.New(r.db).CountSessionItems(ctx, gen.CountSessionItemsParams{
		TenantID: tenantID, SessionID: sql.NullInt64{Int64: sessionID, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("count session items: %w", err)
	}
	return n, nil
}

// CreateItem inserts a line item on a session (session_id set, invoice_id NULL) and
// writes one audit row. in is expected pre-priced by the caller.
func (r *SessionsRepo) CreateItem(ctx context.Context, tenantID, sessionID int64, in billing.LineItemInput) (*billing.LineItem, error) {
	if tenantID == 0 || sessionID == 0 {
		return nil, errors.New("create session item: tenant and session id required")
	}
	if in.Quantity < 0 {
		return nil, errors.New("create session item: quantity must not be negative")
	}
	var newID int64
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: sessionID, Action: "create",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		customItemID, e := billing.ResolveCustomItemID(ctx, q, tenantID, in.CustomItemID)
		if e != nil {
			return e
		}
		row, e := q.CreateLineItem(ctx, lineItemParams(tenantID, &sessionID, customItemID, in))
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
func (r *SessionsRepo) UpdateItem(ctx context.Context, tenantID, itemID int64, in billing.LineItemInput) (*billing.LineItem, error) {
	if tenantID == 0 || itemID == 0 {
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
		customItemID, e := billing.ResolveCustomItemID(ctx, q, tenantID, in.CustomItemID)
		if e != nil {
			return e
		}
		_, e = q.UpdateSessionLineItem(ctx, gen.UpdateSessionLineItemParams{
			ItemID:             db.NullStr(in.ItemID),
			CustomItemID:       customItemID,
			PriceListVersionID: db.NullStr(in.PriceListVersionID),
			Code:               db.NzMaybe(in.Code),
			Description:        in.Description,
			ServiceDate:        db.NzMaybe(in.ServiceDate),
			Unit:               db.NzMaybe(in.Unit),
			StartTime:          db.NzMaybe(in.StartTime),
			EndTime:            db.NzMaybe(in.EndTime),
			Quantity:           in.Quantity,
			UnitPrice:          in.UnitPrice,
			Taxable:            db.B2i(in.Taxable),
			LineTotal:          billing.Round2(in.Quantity * in.UnitPrice),
			TenantID:           tenantID,
			ID:                 itemID,
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
func (r *SessionsRepo) DeleteUnbilledItems(ctx context.Context, tenantID, sessionID int64) error {
	if tenantID == 0 || sessionID == 0 {
		return errors.New("delete unbilled items: tenant and session id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "line_item", EntityID: sessionID, Action: "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteUnbilledItemsForSession(ctx, gen.DeleteUnbilledItemsForSessionParams{
			TenantID: tenantID, SessionID: sql.NullInt64{Int64: sessionID, Valid: true},
		}); err != nil {
			return fmt.Errorf("delete unbilled items: %w", err)
		}
		return nil
	})
}

// DeleteItem removes an UNBILLED session item (invoice_id IS NULL guard) and writes
// one audit row.
func (r *SessionsRepo) DeleteItem(ctx context.Context, tenantID, itemID int64) error {
	if tenantID == 0 || itemID == 0 {
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
// owning session's int id, or (nil, nil) when absent. The session scope ensures an
// item uuid from another session (or tenant) 404s.
func (r *SessionsRepo) GetItemByUUID(ctx context.Context, tenantID, sessionID int64, itemUUID string) (*billing.LineItem, error) {
	if tenantID == 0 || sessionID == 0 {
		return nil, errors.New("get session item: tenant and session id required")
	}
	row, err := gen.New(r.db).GetSessionLineItemByUUID(ctx, gen.GetSessionLineItemByUUIDParams{
		TenantID: tenantID, SessionID: sql.NullInt64{Int64: sessionID, Valid: true}, Uuid: itemUUID,
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
// pre-priced by the caller. The audit EntityID keeps the item's int PK.
func (r *SessionsRepo) UpdateItemByUUID(ctx context.Context, tenantID, sessionID int64, itemUUID string, in billing.LineItemInput) (*billing.LineItem, error) {
	if tenantID == 0 || sessionID == 0 {
		return nil, errors.New("update session item: tenant and session id required")
	}
	if in.Quantity < 0 {
		return nil, errors.New("update session item: quantity must not be negative")
	}
	var item *billing.LineItem
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		customItemID, e := billing.ResolveCustomItemID(ctx, q, tenantID, in.CustomItemID)
		if e != nil {
			return e
		}
		row, e := q.UpdateSessionLineItemByUUID(ctx, gen.UpdateSessionLineItemByUUIDParams{
			ItemID:             db.NullStr(in.ItemID),
			CustomItemID:       customItemID,
			PriceListVersionID: db.NullStr(in.PriceListVersionID),
			Code:               db.NzMaybe(in.Code),
			Description:        in.Description,
			ServiceDate:        db.NzMaybe(in.ServiceDate),
			Unit:               db.NzMaybe(in.Unit),
			StartTime:          db.NzMaybe(in.StartTime),
			EndTime:            db.NzMaybe(in.EndTime),
			Quantity:           in.Quantity,
			UnitPrice:          in.UnitPrice,
			Taxable:            db.B2i(in.Taxable),
			LineTotal:          billing.Round2(in.Quantity * in.UnitPrice),
			TenantID:           tenantID,
			SessionID:          sql.NullInt64{Int64: sessionID, Valid: true},
			Uuid:               itemUUID,
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
		return nil, fmt.Errorf("update session item by uuid: %w", err)
	}
	return item, nil
}

// DeleteItemByUUID removes an UNBILLED session item addressed by uuid (scoped to
// the owning session, invoice_id IS NULL guard) and writes one audit row. A
// missing/billed item is a no-op. The audit EntityID keeps the item's int PK,
// resolved in-tx.
func (r *SessionsRepo) DeleteItemByUUID(ctx context.Context, tenantID, sessionID int64, itemUUID string) error {
	if tenantID == 0 || sessionID == 0 {
		return errors.New("delete session item: tenant and session id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetSessionLineItemByUUID(ctx, gen.GetSessionLineItemByUUIDParams{
			TenantID: tenantID, SessionID: sql.NullInt64{Int64: sessionID, Valid: true}, Uuid: itemUUID,
		})
		if errors.Is(e, sql.ErrNoRows) {
			return nil // missing → no-op
		}
		if e != nil {
			return fmt.Errorf("resolve item: %w", e)
		}
		if err := q.DeleteSessionLineItemByUUID(ctx, gen.DeleteSessionLineItemByUUIDParams{
			TenantID: tenantID, SessionID: sql.NullInt64{Int64: sessionID, Valid: true}, Uuid: itemUUID,
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
		ID: r.ID, Uuid: r.Uuid, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		ItemID: r.ItemID, CustomItemID: r.CustomItemID, CustomItemUuid: db.NullStr(customItemUUID),
		PriceListVersionID: r.PriceListVersionID, Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

// lineItemParams builds the gen insert params for a line item. sessionID nil = an
// invoice-only line; here it is always set (session item, invoice_id NULL). The
// inbound custom-item uuid is resolved to the int FK by the caller and passed in.
func lineItemParams(tenantID int64, sessionID *int64, customItemID sql.NullInt64, in billing.LineItemInput) gen.CreateLineItemParams {
	return gen.CreateLineItemParams{
		Uuid:               ids.New(),
		TenantID:           tenantID,
		SessionID:          db.NullID(sessionID),
		InvoiceID:          sql.NullInt64{}, // unbilled session item
		ItemID:             db.NullStr(in.ItemID),
		CustomItemID:       customItemID,
		PriceListVersionID: db.NullStr(in.PriceListVersionID),
		Code:               db.NzMaybe(in.Code),
		Description:        in.Description,
		ServiceDate:        db.NzMaybe(in.ServiceDate),
		Unit:               db.NzMaybe(in.Unit),
		StartTime:          db.NzMaybe(in.StartTime),
		EndTime:            db.NzMaybe(in.EndTime),
		Quantity:           in.Quantity,
		UnitPrice:          in.UnitPrice,
		Taxable:            db.B2i(in.Taxable),
		LineTotal:          billing.Round2(in.Quantity * in.UnitPrice),
		SortOrder:          sql.NullInt64{Int64: in.SortOrder, Valid: true},
	}
}

// UnbilledByClient aggregates the tenant's recorded-but-unbilled sessions per
// client (count and service-date span), ready for billing suggestions.
func (r *SessionsRepo) UnbilledByClient(ctx context.Context, tenantID int64) ([]UnbilledAgg, error) {
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
		return "", fmt.Errorf("session: marshal tags: %w", err)
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

// sessionFields is the common projection shared by every enriched session read row
// (Get/GetByID/List*). The gen row types are nominally distinct but identical in
// shape; each is adapted into this struct so a single mapper (mapSession) builds
// the DTO. ClientUUID is the joined clients.uuid (NULL if the FK is
// dangling, which the clean schema forbids).
type sessionFields struct {
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

func mapSession(f sessionFields) (*Session, error) {
	tags := []string{}
	if f.tags != "" {
		if err := json.Unmarshal([]byte(f.tags), &tags); err != nil {
			return nil, fmt.Errorf("session %d: unmarshal tags: %w", f.id, err)
		}
		if tags == nil {
			tags = []string{}
		}
	}
	return &Session{
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

// sessionFieldsFromGet / *FromByID / *FromList* adapt the (nominally distinct but
// identically shaped) enriched gen row types into the common sessionFields. They
// keep the per-query row scanners while a single mapper builds the DTO.
func sessionFieldsFromGet(r gen.GetSessionRow) sessionFields {
	return sessionFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromByID(r gen.GetSessionByIDRow) sessionFields {
	return sessionFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromList(r gen.ListSessionsRow) sessionFields {
	return sessionFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromByPart(r gen.ListSessionsByClientRow) sessionFields {
	return sessionFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromByPartRange(r gen.ListSessionsByClientRangeRow) sessionFields {
	return sessionFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromByStatus(r gen.ListSessionsByStatusRow) sessionFields {
	return sessionFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromScheduled(r gen.ListScheduledSessionsRow) sessionFields {
	return sessionFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromRecorded(r gen.ListRecordedUnbilledByClientRow) sessionFields {
	return sessionFields{
		id: r.ID, uuid: r.Uuid, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

// mapSessions maps a slice of enriched gen rows (via an adapter to sessionFields)
// into DTOs. Bounded by len(rows).
func mapSessions[T any](rows []T, adapt func(T) sessionFields) ([]*Session, error) {
	out := make([]*Session, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		s, err := mapSession(adapt(rows[i]))
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
