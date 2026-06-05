package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/numbering"
	"github.com/google/uuid"
)

// Invoice is the domain view of an invoice with its resolved client name and
// embedded line items. Nullable columns are unwrapped to plain values; nullable
// FKs to *int64 (nil when absent). Totals are recomputed server-side on write.
type Invoice struct {
	ID               int64       `json:"id"`
	UUID             string      `json:"uuid"`
	InvoiceNumber    string      `json:"invoiceNumber"`
	ClientID         int64       `json:"clientId"`
	ClientName       string      `json:"clientName"`
	Date             string      `json:"date"`
	DueDate          string      `json:"dueDate"`
	PaymentTerms     string      `json:"paymentTerms"`
	Subtotal         float64     `json:"subtotal"`
	TaxRate          float64     `json:"taxRate"`
	TaxRateID        *int64      `json:"taxRateId"`
	TaxAmount        float64     `json:"taxAmount"`
	Total            float64     `json:"total"`
	Notes            string      `json:"notes"`
	Status           string      `json:"status"`
	CurrencyCode     string      `json:"currencyCode"`
	BusinessSnapshot string      `json:"businessSnapshot"`
	ClientSnapshot   string      `json:"clientSnapshot"`
	PayerSnapshot    string      `json:"payerSnapshot"`
	CreatedAt        string      `json:"createdAt"`
	UpdatedAt        string      `json:"updatedAt"`
	TotalPaid        float64     `json:"totalPaid"`
	Balance          float64     `json:"balance"`
	LineItems        []*LineItem `json:"lineItems"`
}

// LineItem is the domain view of a row in the line_items table.
type LineItem struct {
	ID            int64   `json:"id"`
	UUID          string  `json:"uuid"`
	Description   string  `json:"description"`
	Quantity      float64 `json:"quantity"`
	Rate          float64 `json:"rate"`
	Amount        float64 `json:"amount"`
	Notes         string  `json:"notes"`
	SortOrder     int64   `json:"sortOrder"`
	CatalogItemID *int64  `json:"catalogItemId"`
	RateTierID    *int64  `json:"rateTierId"`
}

// InvoiceInput is the writable subset of an invoice header. Snapshot fields,
// when non-empty, are stored verbatim; when empty, defaults are built from the
// business profile, client and payer.
type InvoiceInput struct {
	ClientID         int64   `json:"clientId"`
	Date             string  `json:"date"`
	DueDate          string  `json:"dueDate"`
	PaymentTerms     string  `json:"paymentTerms"`
	TaxRate          float64 `json:"taxRate"`
	TaxRateID        *int64  `json:"taxRateId"`
	Notes            string  `json:"notes"`
	Status           string  `json:"status"`
	CurrencyCode     string  `json:"currencyCode"`
	BusinessSnapshot string  `json:"businessSnapshot"`
	ClientSnapshot   string  `json:"clientSnapshot"`
	PayerSnapshot    string  `json:"payerSnapshot"`
}

// LineItemInput is the writable subset of a line item; amount is computed.
type LineItemInput struct {
	Description   string  `json:"description"`
	Quantity      float64 `json:"quantity"`
	Rate          float64 `json:"rate"`
	Notes         string  `json:"notes"`
	SortOrder     int64   `json:"sortOrder"`
	CatalogItemID *int64  `json:"catalogItemId"`
	RateTierID    *int64  `json:"rateTierId"`
}

// OverdueInvoice identifies an invoice flipped to overdue by MarkOverdue.
type OverdueInvoice struct {
	ID            int64  `json:"id"`
	InvoiceNumber string `json:"invoiceNumber"`
}

// ClientStats aggregates an individual client's invoice activity.
type ClientStats struct {
	InvoiceCount  int64   `json:"invoiceCount"`
	TotalInvoiced float64 `json:"totalInvoiced"`
	TotalPaid     float64 `json:"totalPaid"`
}

// InvoicesRepo reads and writes the invoices + line_items tables. Creates and
// duplicates allocate an invoice number via the numbering package inside a
// retried transaction; all mutations are audited.
type InvoicesRepo struct {
	db *sql.DB
}

// NewInvoices constructs a repository. A nil db is a programmer error.
func NewInvoices(db *sql.DB) *InvoicesRepo {
	if db == nil {
		panic("repository: NewInvoices requires a non-nil *sql.DB")
	}
	return &InvoicesRepo{db: db}
}

// totals holds the server-computed money fields derived from line items.
type totals struct {
	subtotal  float64
	taxAmount float64
	total     float64
}

// computeTotals sums line amounts and applies the percentage tax rate (e.g. 10
// means 10%). amount = quantity*rate; subtotal = Σ amounts.
func computeTotals(items []LineItemInput, taxRate float64) totals {
	var subtotal float64
	for i := range items { // bounded by len(items)
		subtotal += items[i].Quantity * items[i].Rate
	}
	subtotal = round2(subtotal)
	taxAmount := round2(subtotal * (taxRate / 100))
	return totals{subtotal: subtotal, taxAmount: taxAmount, total: round2(subtotal + taxAmount)}
}

// round2 rounds to two decimal places (cents).
func round2(x float64) float64 {
	return math.Round(x*100) / 100
}

// resolveTaxRate returns the effective tax-rate percentage: in.TaxRate when set,
// else the referenced tax_rates.rate when in.TaxRateID is present and readable.
func (r *InvoicesRepo) resolveTaxRate(ctx context.Context, in InvoiceInput) float64 {
	if in.TaxRate != 0 || in.TaxRateID == nil {
		return in.TaxRate
	}
	tr, err := gen.New(r.db).GetTaxRate(ctx, *in.TaxRateID)
	if err != nil {
		return in.TaxRate
	}
	return tr.Rate
}

// fillSnapshots fills any empty snapshot field on in with a default built from
// the business profile, client and payer.
func (r *InvoicesRepo) fillSnapshots(ctx context.Context, in *InvoiceInput) {
	if in.BusinessSnapshot == "" {
		in.BusinessSnapshot = r.buildBusinessSnapshot(ctx)
	}
	if in.ClientSnapshot == "" {
		in.ClientSnapshot = r.buildClientSnapshot(ctx, in.ClientID)
	}
	if in.PayerSnapshot == "" {
		in.PayerSnapshot = r.buildPayerSnapshot(ctx, in.ClientID)
	}
}

// Create inserts an invoice plus its line items inside one numbering-retried
// transaction, audits the create, and re-reads the row. ClientID and at least
// one line item are required.
func (r *InvoicesRepo) Create(ctx context.Context, in InvoiceInput, items []LineItemInput) (*Invoice, error) {
	if in.ClientID == 0 {
		return nil, errors.New("create invoice: client is required")
	}
	if len(items) == 0 {
		return nil, errors.New("create invoice: at least one line item is required")
	}
	taxRate := r.resolveTaxRate(ctx, in)
	r.fillSnapshots(ctx, &in)

	var newID int64
	err := numbering.WithRetry(ctx, 10, func() error {
		return r.createTx(ctx, in, items, taxRate, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("create invoice: %w", err)
	}
	return r.Get(ctx, newID)
}

// createTx runs a single create attempt: it generates the number, inserts the
// header + items, and logs the audit row, all in one transaction.
func (r *InvoicesRepo) createTx(ctx context.Context, in InvoiceInput, items []LineItemInput, taxRate float64, newID *int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	num, err := numbering.Next(ctx, tx, numbering.Invoice)
	if err != nil {
		return err
	}
	q := gen.New(tx)
	inv, err := q.CreateInvoice(ctx, createParams(in, items, taxRate, num))
	if err != nil {
		return err
	}
	if err := insertItems(ctx, q, inv.ID, items); err != nil {
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

// createParams builds the insert params, applying defaults (draft / USD / custom)
// and computing totals from the line items at the given percentage tax rate.
func createParams(in InvoiceInput, items []LineItemInput, taxRate float64, num string) gen.CreateInvoiceParams {
	t := computeTotals(items, taxRate)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		Uuid:             uuid.NewString(),
		InvoiceNumber:    num,
		ClientID:         in.ClientID,
		Date:             in.Date,
		DueDate:          in.DueDate,
		PaymentTerms:     nz(orDefault(in.PaymentTerms, "custom")),
		Subtotal:         sql.NullFloat64{Float64: t.subtotal, Valid: true},
		TaxRate:          sql.NullFloat64{Float64: taxRate, Valid: true},
		TaxRateID:        nullID(in.TaxRateID),
		TaxAmount:        sql.NullFloat64{Float64: t.taxAmount, Valid: true},
		Total:            sql.NullFloat64{Float64: t.total, Valid: true},
		Notes:            nz(in.Notes),
		Status:           nz(orDefault(in.Status, "draft")),
		CurrencyCode:     nz(orDefault(in.CurrencyCode, "USD")),
		BusinessSnapshot: nz(in.BusinessSnapshot),
		ClientSnapshot:   nz(in.ClientSnapshot),
		PayerSnapshot:    nz(in.PayerSnapshot),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// insertItems writes each line item with its computed amount. Bounded by len.
func insertItems(ctx context.Context, q *gen.Queries, invoiceID int64, items []LineItemInput) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		_, err := q.CreateLineItem(ctx, gen.CreateLineItemParams{
			Uuid:          uuid.NewString(),
			InvoiceID:     invoiceID,
			Description:   it.Description,
			Quantity:      it.Quantity,
			Rate:          it.Rate,
			Amount:        round2(it.Quantity * it.Rate),
			Notes:         nz(it.Notes),
			SortOrder:     nzInt(it.SortOrder),
			CatalogItemID: nullID(it.CatalogItemID),
			RateTierID:    nullID(it.RateTierID),
		})
		if err != nil {
			return fmt.Errorf("insert line item %d: %w", i, err)
		}
	}
	return nil
}

// Get returns the invoice (with client name and line items), or (nil, nil) when
// absent.
func (r *InvoicesRepo) Get(ctx context.Context, id int64) (*Invoice, error) {
	q := gen.New(r.db)
	row, err := q.GetInvoice(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	inv := toInvoiceFromRow(invoiceFieldsFromGet(row))
	rows, err := q.ListLineItems(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list line items: %w", err)
	}
	inv.LineItems = mapLineItems(rows)
	tp, err := q.InvoiceTotalPaid(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("invoice total paid: %w", err)
	}
	inv.TotalPaid = tp
	inv.Balance = round2(inv.Total - tp)
	return inv, nil
}

// List returns every invoice (header only, no line items), ordered newest first.
func (r *InvoicesRepo) List(ctx context.Context) ([]*Invoice, error) {
	rows, err := gen.New(r.db).ListInvoices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	out := make([]*Invoice, 0, len(rows))
	for i := range rows {
		out = append(out, toInvoiceFromRow(invoiceFieldsFromList(rows[i])))
	}
	return out, nil
}

// ListByStatus returns invoices with the given status (header only).
func (r *InvoicesRepo) ListByStatus(ctx context.Context, status string) ([]*Invoice, error) {
	rows, err := gen.New(r.db).ListInvoicesByStatus(ctx, nz(status))
	if err != nil {
		return nil, fmt.Errorf("list invoices by status: %w", err)
	}
	out := make([]*Invoice, 0, len(rows))
	for i := range rows {
		out = append(out, toInvoiceFromRow(invoiceFieldsFromStatus(rows[i])))
	}
	return out, nil
}

// ListClientInvoices returns one client's invoices (header only).
func (r *InvoicesRepo) ListClientInvoices(ctx context.Context, clientID int64) ([]*Invoice, error) {
	rows, err := gen.New(r.db).ListClientInvoices(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("list client invoices: %w", err)
	}
	out := make([]*Invoice, 0, len(rows))
	for i := range rows {
		out = append(out, toInvoiceFromRow(invoiceFieldsFromClient(rows[i])))
	}
	return out, nil
}

// Update rewrites the header (recomputing totals/tax) and replaces all line
// items, atomically with one audit row. Empty snapshot inputs keep the existing
// stored snapshots. Returns (nil, nil) when the invoice does not exist.
func (r *InvoicesRepo) Update(ctx context.Context, id int64, in InvoiceInput, items []LineItemInput) (*Invoice, error) {
	if in.ClientID == 0 {
		return nil, errors.New("update invoice: client is required")
	}
	if len(items) == 0 {
		return nil, errors.New("update invoice: at least one line item is required")
	}
	existing, err := gen.New(r.db).GetInvoice(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update invoice: load existing: %w", err)
	}
	keepSnapshots(&in, existing)
	taxRate := r.resolveTaxRate(ctx, in)

	err = audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "update",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		if _, e := q.UpdateInvoice(ctx, updateParams(in, items, taxRate, id)); e != nil {
			return fmt.Errorf("update: %w", e)
		}
		if e := q.DeleteLineItemsForInvoice(ctx, id); e != nil {
			return fmt.Errorf("clear items: %w", e)
		}
		return insertItems(ctx, q, id, items)
	})
	if err != nil {
		return nil, fmt.Errorf("update invoice: %w", err)
	}
	return r.Get(ctx, id)
}

// keepSnapshots preserves the stored snapshots for any snapshot input left empty.
func keepSnapshots(in *InvoiceInput, existing gen.GetInvoiceRow) {
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

// updateParams builds the update params, recomputing totals from items.
func updateParams(in InvoiceInput, items []LineItemInput, taxRate float64, id int64) gen.UpdateInvoiceParams {
	t := computeTotals(items, taxRate)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpdateInvoiceParams{
		ClientID:         in.ClientID,
		Date:             in.Date,
		DueDate:          in.DueDate,
		PaymentTerms:     nz(orDefault(in.PaymentTerms, "custom")),
		Subtotal:         sql.NullFloat64{Float64: t.subtotal, Valid: true},
		TaxRate:          sql.NullFloat64{Float64: taxRate, Valid: true},
		TaxRateID:        nullID(in.TaxRateID),
		TaxAmount:        sql.NullFloat64{Float64: t.taxAmount, Valid: true},
		Total:            sql.NullFloat64{Float64: t.total, Valid: true},
		Notes:            nz(in.Notes),
		Status:           nz(orDefault(in.Status, "draft")),
		CurrencyCode:     nz(orDefault(in.CurrencyCode, "USD")),
		BusinessSnapshot: nz(in.BusinessSnapshot),
		ClientSnapshot:   nz(in.ClientSnapshot),
		PayerSnapshot:    nz(in.PayerSnapshot),
		UpdatedAt:        now,
		ID:               id,
	}
}

// UpdateStatus sets just the status column, atomically with one audit row.
func (r *InvoicesRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if e := gen.New(tx).UpdateInvoiceStatus(ctx, gen.UpdateInvoiceStatusParams{
			Status: nz(status), UpdatedAt: now, ID: id,
		}); e != nil {
			return fmt.Errorf("update status: %w", e)
		}
		return nil
	})
}

// Delete removes an invoice (line items cascade) and writes one audit row.
func (r *InvoicesRepo) Delete(ctx context.Context, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).DeleteInvoice(ctx, id); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return nil
	})
}

// BulkDelete removes several invoices and writes one audit row. Empty is a no-op.
func (r *InvoicesRepo) BulkDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if e := q.DeleteInvoice(ctx, id); e != nil {
				return fmt.Errorf("delete %d: %w", id, e)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "invoice", EntityID: 0, Action: "bulk_delete",
			Changes: audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// BulkUpdateStatus sets the status of several invoices and writes one audit row.
func (r *InvoicesRepo) BulkUpdateStatus(ctx context.Context, ids []int64, status string) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		now := time.Now().UTC().Format(time.RFC3339)
		for _, id := range ids { // bounded by len(ids)
			if e := q.UpdateInvoiceStatus(ctx, gen.UpdateInvoiceStatusParams{
				Status: nz(status), UpdatedAt: now, ID: id,
			}); e != nil {
				return fmt.Errorf("status %d: %w", id, e)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "invoice", EntityID: 0, Action: "bulk_status",
			Changes: audit.Changes(map[string]any{"ids": ids, "status": status}),
		})
	})
}

// Duplicate creates a new draft invoice copying the source's client, tax rate,
// notes, currency, snapshots and line items, but resetting the date to today,
// clearing the due date and tax-rate id, and assigning a fresh invoice number.
func (r *InvoicesRepo) Duplicate(ctx context.Context, id int64) (*Invoice, error) {
	src, err := r.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("duplicate invoice: %w", err)
	}
	if src == nil {
		return nil, errors.New("duplicate invoice: source not found")
	}
	in := InvoiceInput{
		ClientID:         src.ClientID,
		Date:             time.Now().UTC().Format("2006-01-02"),
		DueDate:          "",
		PaymentTerms:     "custom",
		TaxRate:          src.TaxRate,
		TaxRateID:        nil,
		Notes:            src.Notes,
		Status:           "draft",
		CurrencyCode:     src.CurrencyCode,
		BusinessSnapshot: src.BusinessSnapshot,
		ClientSnapshot:   src.ClientSnapshot,
		PayerSnapshot:    src.PayerSnapshot,
	}
	items := lineItemsToInput(src.LineItems)

	var newID int64
	err = numbering.WithRetry(ctx, 10, func() error {
		return r.createTx(ctx, in, items, src.TaxRate, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("duplicate invoice: %w", err)
	}
	return r.Get(ctx, newID)
}

// lineItemsToInput converts stored line items back into writable inputs.
func lineItemsToInput(items []*LineItem) []LineItemInput {
	out := make([]LineItemInput, 0, len(items))
	for i := range items { // bounded by len(items)
		it := items[i]
		out = append(out, LineItemInput{
			Description:   it.Description,
			Quantity:      it.Quantity,
			Rate:          it.Rate,
			Notes:         it.Notes,
			SortOrder:     it.SortOrder,
			CatalogItemID: it.CatalogItemID,
			RateTierID:    it.RateTierID,
		})
	}
	return out
}

// MarkOverdue flips every 'sent' invoice whose due date has passed to 'overdue',
// auditing each, atomically. Returns the affected invoices (empty when none).
func (r *InvoicesRepo) MarkOverdue(ctx context.Context) ([]OverdueInvoice, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mark overdue: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	rows, err := q.SelectOverdueInvoices(ctx)
	if err != nil {
		return nil, fmt.Errorf("mark overdue: select: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]OverdueInvoice, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		if e := flipOverdue(ctx, tx, q, rows[i].ID, now); e != nil {
			return nil, fmt.Errorf("mark overdue: %w", e)
		}
		out = append(out, OverdueInvoice{ID: rows[i].ID, InvoiceNumber: rows[i].InvoiceNumber})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("mark overdue: commit: %w", err)
	}
	return out, nil
}

// flipOverdue sets one invoice to overdue and logs the transition.
func flipOverdue(ctx context.Context, tx *sql.Tx, q *gen.Queries, id int64, now string) error {
	if e := q.UpdateInvoiceStatus(ctx, gen.UpdateInvoiceStatusParams{
		Status: nz("overdue"), UpdatedAt: now, ID: id,
	}); e != nil {
		return e
	}
	return audit.Log(ctx, tx, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"from": "sent", "to": "overdue"}),
	})
}

// ClientStats returns the count and summed total of a client's invoices.
func (r *InvoicesRepo) ClientStats(ctx context.Context, clientID int64) (*ClientStats, error) {
	row, err := gen.New(r.db).ClientInvoiceStats(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("client stats: %w", err)
	}
	return &ClientStats{InvoiceCount: row.InvoiceCount, TotalInvoiced: row.TotalInvoiced, TotalPaid: row.TotalPaid}, nil
}

// snapshot builds the default snapshot JSON for an entity. metadata is parsed
// into an object (or {} on failure) so the stored shape is uniform.
func snapshotJSON(name, email, phone, address, metadata string) string {
	var meta any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil || metadata == "" {
		meta = map[string]any{}
	}
	b, err := json.Marshal(map[string]any{
		"name": name, "email": email, "phone": phone, "address": address, "metadata": meta,
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// buildBusinessSnapshot reads the business profile and renders a default snapshot.
func (r *InvoicesRepo) buildBusinessSnapshot(ctx context.Context) string {
	bp, err := gen.New(r.db).GetBusinessProfile(ctx)
	if err != nil {
		return "{}"
	}
	return snapshotJSON(bp.Name, bp.Email.String, bp.Phone.String, bp.Address.String, bp.Metadata.String)
}

// buildClientSnapshot reads the client and renders a default snapshot.
func (r *InvoicesRepo) buildClientSnapshot(ctx context.Context, clientID int64) string {
	c, err := gen.New(r.db).GetClient(ctx, clientID)
	if err != nil {
		return "{}"
	}
	return snapshotJSON(c.Name, c.Email.String, c.Phone.String, c.Address.String, c.Metadata.String)
}

// buildPayerSnapshot resolves the client's payer (if any) and renders a default
// snapshot. Returns "{}" when the client has no payer.
func (r *InvoicesRepo) buildPayerSnapshot(ctx context.Context, clientID int64) string {
	q := gen.New(r.db)
	c, err := q.GetClient(ctx, clientID)
	if err != nil || !c.PayerID.Valid {
		return "{}"
	}
	p, err := q.GetPayer(ctx, c.PayerID.Int64)
	if err != nil {
		return "{}"
	}
	return snapshotJSON(p.Name, p.Email.String, p.Phone.String, p.Address.String, p.Metadata.String)
}

// invoiceFields is the shared, flat shape of every invoices join row (List,
// ListByStatus, ListClientInvoices and Get all produce identical structs under
// distinct gen type names, each adding ClientName).
type invoiceFields struct {
	id                                  int64
	uuid, invoiceNumber                 string
	clientID                            int64
	date, dueDate                       string
	paymentTerms                        sql.NullString
	subtotal, taxRate                   sql.NullFloat64
	taxRateID                           sql.NullInt64
	taxAmount, total                    sql.NullFloat64
	notes, status, currencyCode         sql.NullString
	businessSnap, clientSnap, payerSnap sql.NullString
	createdAt, updatedAt                string
	clientName                          sql.NullString
}

// toInvoiceFromRow builds a domain Invoice (without line items) from the
// unwrapped join columns. LineItems defaults to a non-nil empty slice.
func toInvoiceFromRow(f invoiceFields) *Invoice {
	return &Invoice{
		ID:               f.id,
		UUID:             f.uuid,
		InvoiceNumber:    f.invoiceNumber,
		ClientID:         f.clientID,
		ClientName:       f.clientName.String,
		Date:             f.date,
		DueDate:          f.dueDate,
		PaymentTerms:     f.paymentTerms.String,
		Subtotal:         f.subtotal.Float64,
		TaxRate:          f.taxRate.Float64,
		TaxRateID:        ptrID(f.taxRateID),
		TaxAmount:        f.taxAmount.Float64,
		Total:            f.total.Float64,
		Notes:            f.notes.String,
		Status:           f.status.String,
		CurrencyCode:     f.currencyCode.String,
		BusinessSnapshot: f.businessSnap.String,
		ClientSnapshot:   f.clientSnap.String,
		PayerSnapshot:    f.payerSnap.String,
		CreatedAt:        f.createdAt,
		UpdatedAt:        f.updatedAt,
		LineItems:        []*LineItem{},
	}
}

func invoiceFieldsFromGet(r gen.GetInvoiceRow) invoiceFields {
	return invoiceFields{
		id: r.ID, uuid: r.Uuid, invoiceNumber: r.InvoiceNumber, clientID: r.ClientID,
		date: r.Date, dueDate: r.DueDate, paymentTerms: r.PaymentTerms,
		subtotal: r.Subtotal, taxRate: r.TaxRate, taxRateID: r.TaxRateID,
		taxAmount: r.TaxAmount, total: r.Total,
		notes: r.Notes, status: r.Status, currencyCode: r.CurrencyCode,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

func invoiceFieldsFromList(r gen.ListInvoicesRow) invoiceFields {
	return invoiceFields{
		id: r.ID, uuid: r.Uuid, invoiceNumber: r.InvoiceNumber, clientID: r.ClientID,
		date: r.Date, dueDate: r.DueDate, paymentTerms: r.PaymentTerms,
		subtotal: r.Subtotal, taxRate: r.TaxRate, taxRateID: r.TaxRateID,
		taxAmount: r.TaxAmount, total: r.Total,
		notes: r.Notes, status: r.Status, currencyCode: r.CurrencyCode,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

func invoiceFieldsFromStatus(r gen.ListInvoicesByStatusRow) invoiceFields {
	return invoiceFields{
		id: r.ID, uuid: r.Uuid, invoiceNumber: r.InvoiceNumber, clientID: r.ClientID,
		date: r.Date, dueDate: r.DueDate, paymentTerms: r.PaymentTerms,
		subtotal: r.Subtotal, taxRate: r.TaxRate, taxRateID: r.TaxRateID,
		taxAmount: r.TaxAmount, total: r.Total,
		notes: r.Notes, status: r.Status, currencyCode: r.CurrencyCode,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

func invoiceFieldsFromClient(r gen.ListClientInvoicesRow) invoiceFields {
	return invoiceFields{
		id: r.ID, uuid: r.Uuid, invoiceNumber: r.InvoiceNumber, clientID: r.ClientID,
		date: r.Date, dueDate: r.DueDate, paymentTerms: r.PaymentTerms,
		subtotal: r.Subtotal, taxRate: r.TaxRate, taxRateID: r.TaxRateID,
		taxAmount: r.TaxAmount, total: r.Total,
		notes: r.Notes, status: r.Status, currencyCode: r.CurrencyCode,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

// mapLineItems maps generated line item rows to domain line items (non-nil).
func mapLineItems(rows []gen.LineItem) []*LineItem {
	out := make([]*LineItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toLineItem(rows[i]))
	}
	return out
}

// toLineItem maps one generated line item to the domain shape.
func toLineItem(row gen.LineItem) *LineItem {
	return &LineItem{
		ID:            row.ID,
		UUID:          row.Uuid,
		Description:   row.Description,
		Quantity:      row.Quantity,
		Rate:          row.Rate,
		Amount:        row.Amount,
		Notes:         row.Notes.String,
		SortOrder:     row.SortOrder.Int64,
		CatalogItemID: ptrID(row.CatalogItemID),
		RateTierID:    ptrID(row.RateTierID),
	}
}

// orDefault returns s when non-empty, otherwise def.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
