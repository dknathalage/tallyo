package invoice

// NOTE (J4): rewritten to the invoice/line-item domain (spec §4.2). The
// header no longer carries payment_terms / currency / tax_rate / tax_rate_id;
// it carries client_id, optional payer_id, and subtotal/tax/total.
// Line items carry catalogue fields: code, service_date, unit, unit_price, taxable,
// line_total, and optional item_id / custom_item_id / price_list_version_id.
//
// Design decisions (deferred concerns belong to J8/J10):
//   - `tax` is supplied on the header input (computed upstream by the J10
//     validation engine). This repo only sums line totals → subtotal and
//     subtotal+tax → total, rounding to the cent at each boundary (spec §6 money
//     note). It does NOT perform price-cap / plan-window validation (J10).
//   - Per-tenant document numbering: the tenant-scoped gen.MaxInvoiceNumberLike
//     query (WHERE tenant_id = ?) reads the current max suffix inside the create
//     tx; numbering.Format builds the next number; numbering.WithRetry wraps the
//     tx so a UNIQUE(tenant_id, number) collision from a concurrent creator is
//     retried. This is the single numbering implementation (J11 consolidation).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/numbering"
)

// Invoice is the domain view of an invoice with its resolved client name
// and embedded line items.
type Invoice struct {
	ID               string              `json:"id"` // public identifier (invoice uuid)
	Number           string              `json:"number"`
	ClientID         string              `json:"-"`        // internal FK; the public ref is clientId (uuid)
	ClientUUID       string              `json:"clientId"` // client uuid
	ClientName       string              `json:"clientName"`
	PayerUUID        *string             `json:"payerId"` // payer uuid (nil when none)
	Status           string              `json:"status"`
	IssueDate        string              `json:"issueDate"`
	DueDate          string              `json:"dueDate"`
	Subtotal         float64             `json:"subtotal"`
	Tax              float64             `json:"tax"`
	Total            float64             `json:"total"`
	Notes            string              `json:"notes"`
	BusinessSnapshot string              `json:"businessSnapshot"`
	ClientSnapshot   string              `json:"clientSnapshot"`
	PayerSnapshot    string              `json:"payerSnapshot"`
	CreatedAt        string              `json:"createdAt"`
	UpdatedAt        string              `json:"updatedAt"`
	TotalPaid        float64             `json:"totalPaid"`
	Balance          float64             `json:"balance"`
	LineItems        []*billing.LineItem `json:"lineItems"`
}

// InvoiceInput is the writable subset of an invoice header. Snapshot fields,
// when non-empty, are stored verbatim; when empty, defaults are built from the
// business profile, client and payer.
type InvoiceInput struct {
	ClientID         string  `json:"clientId"`
	PayerID          *string `json:"payerId"`
	Status           string  `json:"status"`
	IssueDate        string  `json:"issueDate"`
	DueDate          string  `json:"dueDate"`
	Tax              float64 `json:"tax"`
	Notes            string  `json:"notes"`
	BusinessSnapshot string  `json:"businessSnapshot"`
	ClientSnapshot   string  `json:"clientSnapshot"`
	PayerSnapshot    string  `json:"payerSnapshot"`
}

// ClientStats aggregates a client's invoice activity.
type ClientStats struct {
	InvoiceCount  int64   `json:"invoiceCount"`
	TotalInvoiced float64 `json:"totalInvoiced"`
	TotalPaid     float64 `json:"totalPaid"`
}

// InvoicesRepo reads and writes the invoices + line_items tables (tenant-scoped).
type InvoicesRepo struct {
	db   db.Executor
	snap *billing.SnapshotBuilder
}

// NewInvoices constructs a repository. A nil db is a programmer error.
func NewInvoices(db db.Executor) *InvoicesRepo {
	if db == nil {
		panic("invoice: NewInvoices requires a non-nil *sql.DB")
	}
	return &InvoicesRepo{db: db, snap: billing.NewSnapshotBuilder(db)}
}

// fillSnapshots fills any empty snapshot field on in with a default built from
// the business profile, client and payer.
func (r *InvoicesRepo) fillSnapshots(ctx context.Context, tenantID string, in *InvoiceInput) {
	if in.BusinessSnapshot == "" {
		in.BusinessSnapshot = r.snap.Business(ctx, tenantID)
	}
	if in.ClientSnapshot == "" {
		in.ClientSnapshot = r.snap.Client(ctx, tenantID, in.ClientID)
	}
	if in.PayerSnapshot == "" {
		in.PayerSnapshot = r.snap.Payer(ctx, tenantID, in.PayerID)
	}
}

// Create inserts an invoice plus its line items inside one numbering-retried
// transaction, audits the create, and re-reads the row. ClientID and at
// least one line item are required.
func (r *InvoicesRepo) Create(ctx context.Context, tenantID string, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
	if tenantID == "" {
		return nil, errors.New("create invoice: tenant id required")
	}
	if in.ClientID == "" {
		return nil, errors.New("create invoice: client is required")
	}
	if len(items) == 0 {
		return nil, errors.New("create invoice: at least one line item is required")
	}
	r.fillSnapshots(ctx, tenantID, &in)

	var newID string
	err := numbering.WithRetry(ctx, 10, func() error {
		return r.createTx(ctx, tenantID, in, items, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("create invoice: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// createTx runs a single create attempt: it allocates the per-tenant number,
// inserts the header + items, and logs the audit row, all in one transaction.
func (r *InvoicesRepo) createTx(ctx context.Context, tenantID string, in InvoiceInput, items []billing.LineItemInput, newID *string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	num, err := NextInvoiceNumber(ctx, q, tenantID)
	if err != nil {
		return err
	}
	inv, err := q.CreateInvoice(ctx, createInvoiceParams(tenantID, in, items, num))
	if err != nil {
		return err
	}
	if err := billing.InsertLineItems(ctx, q, tenantID, inv.ID, items); err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Entry{
		EntityType: "invoice", EntityID: inv.ID, Action: "create",
		Changes: audit.Changes(map[string]any{"number": num}),
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*newID = inv.ID
	return nil
}

// NextInvoiceNumber allocates the next per-tenant invoice number ("INV-NNNN").
// The shared mechanic now lives in billing; this thin wrapper keeps invoice's
// own call sites unchanged and pins the invoice prefix.
func NextInvoiceNumber(ctx context.Context, q *gen.Queries, tenantID string) (string, error) {
	return billing.NextNumber(ctx, q, tenantID, "INV-")
}

// createInvoiceParams builds the insert params, applying defaults (draft) and
// computing totals from the line items.
func createInvoiceParams(tenantID string, in InvoiceInput, items []billing.LineItemInput, num string) gen.CreateInvoiceParams {
	t := billing.ComputeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		ID:               ids.New(),
		TenantID:         tenantID,
		Number:           num,
		ClientID:         in.ClientID,
		PayerID:          db.NullStr(in.PayerID),
		Status:           orDefault(in.Status, "draft"),
		IssueDate:        in.IssueDate,
		DueDate:          in.DueDate,
		Subtotal:         t.Subtotal,
		Tax:              t.Tax,
		Total:            t.Total,
		Notes:            db.NzMaybe(in.Notes),
		BusinessSnapshot: db.NzMaybe(in.BusinessSnapshot),
		ClientSnapshot:   db.NzMaybe(in.ClientSnapshot),
		PayerSnapshot:    db.NzMaybe(in.PayerSnapshot),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// Get returns the invoice (with client name and line items), or (nil, nil)
// when absent.
func (r *InvoicesRepo) Get(ctx context.Context, tenantID, id string) (*Invoice, error) {
	q := gen.New(r.db)
	row, err := q.GetInvoiceByID(ctx, gen.GetInvoiceByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	inv := toInvoiceFromRow(invoiceFieldsFromGetByID(row))
	return r.enrichInvoice(ctx, q, tenantID, inv)
}

// GetByUUID returns the invoice (with line items) addressed by its uuid, or
// (nil, nil) when no invoice matches the uuid for the tenant. Public HTTP read.
func (r *InvoicesRepo) GetByUUID(ctx context.Context, tenantID string, invoiceUUID string) (*Invoice, error) {
	q := gen.New(r.db)
	row, err := q.GetInvoice(ctx, gen.GetInvoiceParams{TenantID: tenantID, ID: invoiceUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get invoice by uuid: %w", err)
	}
	inv := toInvoiceFromRow(invoiceFieldsFromGet(row))
	return r.enrichInvoice(ctx, q, tenantID, inv)
}

// enrichInvoice loads an invoice's line items and total-paid/balance. The
// invoice's row id (inv.ID) keys the lookups.
func (r *InvoicesRepo) enrichInvoice(ctx context.Context, q *gen.Queries, tenantID string, inv *Invoice) (*Invoice, error) {
	rows, err := q.ListLineItemsForInvoice(ctx, gen.ListLineItemsForInvoiceParams{TenantID: tenantID, InvoiceID: sql.NullString{String: inv.ID, Valid: true}})
	if err != nil {
		return nil, fmt.Errorf("list line items: %w", err)
	}
	inv.LineItems = mapLineItems(rows)
	tp, err := q.InvoiceTotalPaid(ctx, gen.InvoiceTotalPaidParams{TenantID: tenantID, InvoiceID: inv.ID})
	if err != nil {
		return nil, fmt.Errorf("invoice total paid: %w", err)
	}
	inv.TotalPaid = tp
	inv.Balance = billing.Round2(inv.Total - tp)
	return inv, nil
}

// ResolveInvoiceID resolves an invoice uuid to its row id (uuid), scoped to the
// tenant. Returns ("", nil) when no invoice matches the uuid (caller 404s).
func (r *InvoicesRepo) ResolveInvoiceID(ctx context.Context, tenantID string, invoiceUUID string) (string, error) {
	id, err := gen.New(r.db).GetInvoiceIDByUUID(ctx, gen.GetInvoiceIDByUUIDParams{TenantID: tenantID, ID: invoiceUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve invoice uuid: %w", err)
	}
	return id, nil
}

// Update rewrites the header (recomputing totals) and replaces all line items,
// atomically with one audit row. Empty snapshot inputs keep the existing stored
// snapshots. Returns (nil, nil) when the invoice does not exist.
func (r *InvoicesRepo) Update(ctx context.Context, tenantID, id string, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
	if in.ClientID == "" {
		return nil, errors.New("update invoice: client is required")
	}
	if len(items) == 0 {
		return nil, errors.New("update invoice: at least one line item is required")
	}
	existing, err := gen.New(r.db).GetInvoiceByID(ctx, gen.GetInvoiceByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update invoice: load existing: %w", err)
	}
	keepSnapshots(&in, existing)

	err = audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "update",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		if _, e := q.UpdateInvoice(ctx, updateInvoiceParams(tenantID, in, items, existing.Number, id)); e != nil {
			return fmt.Errorf("update: %w", e)
		}
		if e := q.DeleteLineItemsForInvoice(ctx, gen.DeleteLineItemsForInvoiceParams{TenantID: tenantID, InvoiceID: sql.NullString{String: id, Valid: true}}); e != nil {
			return fmt.Errorf("clear items: %w", e)
		}
		return billing.InsertLineItems(ctx, q, tenantID, id, items)
	})
	if err != nil {
		return nil, fmt.Errorf("update invoice: %w", err)
	}
	return r.Get(ctx, tenantID, id)
}

// keepSnapshots preserves the stored snapshots for any snapshot input left empty.
func keepSnapshots(in *InvoiceInput, existing gen.GetInvoiceByIDRow) {
	if in.BusinessSnapshot == "" {
		in.BusinessSnapshot = existing.BusinessSnapshot.String
	}
	if in.ClientSnapshot == "" {
		in.ClientSnapshot = existing.ClientSnapshot.String
	}
	if in.PayerSnapshot == "" {
		in.PayerSnapshot = existing.PayerSnapshot.String
	}
}

// updateInvoiceParams builds the update params, recomputing totals from items.
// The document number is immutable, so the existing number is preserved.
func updateInvoiceParams(tenantID string, in InvoiceInput, items []billing.LineItemInput, number string, id string) gen.UpdateInvoiceParams {
	t := billing.ComputeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpdateInvoiceParams{
		Number:           number,
		ClientID:         in.ClientID,
		PayerID:          db.NullStr(in.PayerID),
		Status:           orDefault(in.Status, "draft"),
		IssueDate:        in.IssueDate,
		DueDate:          in.DueDate,
		Subtotal:         t.Subtotal,
		Tax:              t.Tax,
		Total:            t.Total,
		Notes:            db.NzMaybe(in.Notes),
		BusinessSnapshot: db.NzMaybe(in.BusinessSnapshot),
		ClientSnapshot:   db.NzMaybe(in.ClientSnapshot),
		PayerSnapshot:    db.NzMaybe(in.PayerSnapshot),
		UpdatedAt:        now,
		TenantID:         tenantID,
		ID:               id,
	}
}

// Delete removes an invoice and writes one audit row. Session items are unlinked
// (invoice_id→NULL) BEFORE the delete so the line_items.invoice_id ON DELETE
// CASCADE removes only session-less manual lines; session items survive (session_id
// intact) and return to their session. Unlink + cascade are atomic in one tx.
func (r *InvoicesRepo) Delete(ctx context.Context, tenantID, id string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		if e := q.UnlinkSessionItemsFromInvoice(ctx, gen.UnlinkSessionItemsFromInvoiceParams{
			TenantID: tenantID, InvoiceID: sql.NullString{String: id, Valid: true},
		}); e != nil {
			return fmt.Errorf("unlink session items: %w", e)
		}
		if e := q.DeleteInvoice(ctx, gen.DeleteInvoiceParams{TenantID: tenantID, ID: id}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return nil
	})
}

// Exists reports whether the tenant has an invoice with the given id.
// It satisfies the session.InvoiceChecker interface so the session service can
// verify invoice ownership without importing the invoice package.
func (r *InvoicesRepo) Exists(ctx context.Context, tenantID, invoiceID string) (bool, error) {
	inv, err := r.Get(ctx, tenantID, invoiceID)
	if err != nil {
		return false, err
	}
	return inv != nil, nil
}
