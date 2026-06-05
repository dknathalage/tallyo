package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/numbering"
	"github.com/google/uuid"
)

// ErrNotAccepted is returned when converting an estimate that is not in the
// 'accepted' state.
var ErrNotAccepted = errors.New("only accepted estimates can be converted")

// ErrAlreadyConverted is returned when converting an estimate that already has
// a linked invoice.
var ErrAlreadyConverted = errors.New("estimate already converted")

// Estimate is the domain view of an estimate with its resolved client name and
// embedded line items. Mirrors Invoice with estimate-specific deltas:
// valid_until replaces due_date, there is no payment_terms, and an optional
// converted_invoice_id records the invoice produced by Convert.
type Estimate struct {
	ID                 int64               `json:"id"`
	UUID               string              `json:"uuid"`
	EstimateNumber     string              `json:"estimateNumber"`
	ClientID           int64               `json:"clientId"`
	ClientName         string              `json:"clientName"`
	Date               string              `json:"date"`
	ValidUntil         string              `json:"validUntil"`
	Subtotal           float64             `json:"subtotal"`
	TaxRate            float64             `json:"taxRate"`
	TaxRateID          *int64              `json:"taxRateId"`
	TaxAmount          float64             `json:"taxAmount"`
	Total              float64             `json:"total"`
	Notes              string              `json:"notes"`
	Status             string              `json:"status"`
	CurrencyCode       string              `json:"currencyCode"`
	ConvertedInvoiceID *int64              `json:"convertedInvoiceId"`
	BusinessSnapshot   string              `json:"businessSnapshot"`
	ClientSnapshot     string              `json:"clientSnapshot"`
	PayerSnapshot      string              `json:"payerSnapshot"`
	CreatedAt          string              `json:"createdAt"`
	UpdatedAt          string              `json:"updatedAt"`
	LineItems          []*EstimateLineItem `json:"lineItems"`
}

// EstimateLineItem is the domain view of a row in estimate_line_items; it has
// the same shape as LineItem.
type EstimateLineItem struct {
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

// EstimateInput is the writable subset of an estimate header. Snapshot fields,
// when non-empty, are stored verbatim; when empty, defaults are built from the
// business profile, client and payer.
type EstimateInput struct {
	ClientID         int64   `json:"clientId"`
	Date             string  `json:"date"`
	ValidUntil       string  `json:"validUntil"`
	TaxRate          float64 `json:"taxRate"`
	TaxRateID        *int64  `json:"taxRateId"`
	Notes            string  `json:"notes"`
	Status           string  `json:"status"`
	CurrencyCode     string  `json:"currencyCode"`
	BusinessSnapshot string  `json:"businessSnapshot"`
	ClientSnapshot   string  `json:"clientSnapshot"`
	PayerSnapshot    string  `json:"payerSnapshot"`
}

// ConvertResult identifies the invoice produced by Convert.
type ConvertResult struct {
	InvoiceID      int64  `json:"invoiceId"`
	InvoiceNumber  string `json:"invoiceNumber"`
	EstimateNumber string `json:"estimateNumber"`
}

// EstimatesRepo reads and writes the estimates + estimate_line_items tables.
// Creates and duplicates allocate an estimate number, and Convert allocates an
// invoice number, all via the numbering package inside retried transactions;
// every mutation is audited.
type EstimatesRepo struct {
	db   *sql.DB
	snap *InvoicesRepo // reused for the shared snapshot builders
}

// NewEstimates constructs a repository. A nil db is a programmer error.
func NewEstimates(db *sql.DB) *EstimatesRepo {
	if db == nil {
		panic("repository: NewEstimates requires a non-nil *sql.DB")
	}
	return &EstimatesRepo{db: db, snap: NewInvoices(db)}
}

// resolveTaxRate returns the effective tax-rate percentage: in.TaxRate when set,
// else the referenced tax_rates.rate when in.TaxRateID is present and readable.
func (r *EstimatesRepo) resolveTaxRate(ctx context.Context, in EstimateInput) float64 {
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
func (r *EstimatesRepo) fillSnapshots(ctx context.Context, in *EstimateInput) {
	if in.BusinessSnapshot == "" {
		in.BusinessSnapshot = r.snap.buildBusinessSnapshot(ctx)
	}
	if in.ClientSnapshot == "" {
		in.ClientSnapshot = r.snap.buildClientSnapshot(ctx, in.ClientID)
	}
	if in.PayerSnapshot == "" {
		in.PayerSnapshot = r.snap.buildPayerSnapshot(ctx, in.ClientID)
	}
}

// Create inserts an estimate plus its line items inside one numbering-retried
// transaction, audits the create, and re-reads the row. ClientID and at least
// one line item are required.
func (r *EstimatesRepo) Create(ctx context.Context, in EstimateInput, items []LineItemInput) (*Estimate, error) {
	if in.ClientID == 0 {
		return nil, errors.New("create estimate: client is required")
	}
	if len(items) == 0 {
		return nil, errors.New("create estimate: at least one line item is required")
	}
	taxRate := r.resolveTaxRate(ctx, in)
	r.fillSnapshots(ctx, &in)

	var newID int64
	err := numbering.WithRetry(ctx, 10, func() error {
		return r.createTx(ctx, in, items, taxRate, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("create estimate: %w", err)
	}
	return r.Get(ctx, newID)
}

// createTx runs a single create attempt: it generates the number, inserts the
// header + items, and logs the audit row, all in one transaction.
func (r *EstimatesRepo) createTx(ctx context.Context, in EstimateInput, items []LineItemInput, taxRate float64, newID *int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	num, err := numbering.Next(ctx, tx, numbering.Estimate)
	if err != nil {
		return err
	}
	q := gen.New(tx)
	est, err := q.CreateEstimate(ctx, estimateCreateParams(in, items, taxRate, num))
	if err != nil {
		return err
	}
	if err := insertEstimateItems(ctx, q, est.ID, items); err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Entry{
		EntityType: "estimate", EntityID: est.ID, Action: "create",
		Changes: audit.Changes(map[string]any{"number": num}),
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*newID = est.ID
	return nil
}

// estimateCreateParams builds the insert params, applying defaults (draft / USD)
// and computing totals from the line items at the given percentage tax rate.
func estimateCreateParams(in EstimateInput, items []LineItemInput, taxRate float64, num string) gen.CreateEstimateParams {
	t := computeTotals(items, taxRate)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateEstimateParams{
		Uuid:             uuid.NewString(),
		EstimateNumber:   num,
		ClientID:         nzInt(in.ClientID),
		Date:             in.Date,
		ValidUntil:       in.ValidUntil,
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

// insertEstimateItems writes each line item with its computed amount.
func insertEstimateItems(ctx context.Context, q *gen.Queries, estimateID int64, items []LineItemInput) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		_, err := q.CreateEstimateLineItem(ctx, gen.CreateEstimateLineItemParams{
			Uuid:          uuid.NewString(),
			EstimateID:    estimateID,
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
			return fmt.Errorf("insert estimate line item %d: %w", i, err)
		}
	}
	return nil
}

// Get returns the estimate (with client name and line items), or (nil, nil)
// when absent.
func (r *EstimatesRepo) Get(ctx context.Context, id int64) (*Estimate, error) {
	q := gen.New(r.db)
	row, err := q.GetEstimate(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get estimate: %w", err)
	}
	est := toEstimateFromRow(estimateFieldsFromGet(row))
	rows, err := q.ListEstimateLineItems(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list estimate line items: %w", err)
	}
	est.LineItems = mapEstimateLineItems(rows)
	return est, nil
}

// List returns every estimate (header only, no line items), newest first.
func (r *EstimatesRepo) List(ctx context.Context) ([]*Estimate, error) {
	rows, err := gen.New(r.db).ListEstimates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list estimates: %w", err)
	}
	out := make([]*Estimate, 0, len(rows))
	for i := range rows {
		out = append(out, toEstimateFromRow(estimateFieldsFromList(rows[i])))
	}
	return out, nil
}

// ListByStatus returns estimates with the given status (header only).
func (r *EstimatesRepo) ListByStatus(ctx context.Context, status string) ([]*Estimate, error) {
	rows, err := gen.New(r.db).ListEstimatesByStatus(ctx, nz(status))
	if err != nil {
		return nil, fmt.Errorf("list estimates by status: %w", err)
	}
	out := make([]*Estimate, 0, len(rows))
	for i := range rows {
		out = append(out, toEstimateFromRow(estimateFieldsFromStatus(rows[i])))
	}
	return out, nil
}

// ListClientEstimates returns one client's estimates (header only).
func (r *EstimatesRepo) ListClientEstimates(ctx context.Context, clientID int64) ([]*Estimate, error) {
	rows, err := gen.New(r.db).ListClientEstimates(ctx, nzInt(clientID))
	if err != nil {
		return nil, fmt.Errorf("list client estimates: %w", err)
	}
	out := make([]*Estimate, 0, len(rows))
	for i := range rows {
		out = append(out, toEstimateFromRow(estimateFieldsFromClient(rows[i])))
	}
	return out, nil
}

// Update rewrites the header (recomputing totals/tax) and replaces all line
// items, atomically with one audit row. Empty snapshot inputs keep the existing
// stored snapshots. Returns (nil, nil) when the estimate does not exist.
func (r *EstimatesRepo) Update(ctx context.Context, id int64, in EstimateInput, items []LineItemInput) (*Estimate, error) {
	if in.ClientID == 0 {
		return nil, errors.New("update estimate: client is required")
	}
	if len(items) == 0 {
		return nil, errors.New("update estimate: at least one line item is required")
	}
	existing, err := gen.New(r.db).GetEstimate(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update estimate: load existing: %w", err)
	}
	keepEstimateSnapshots(&in, existing)
	taxRate := r.resolveTaxRate(ctx, in)

	err = audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "estimate", EntityID: id, Action: "update",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		if _, e := q.UpdateEstimate(ctx, estimateUpdateParams(in, items, taxRate, id)); e != nil {
			return fmt.Errorf("update: %w", e)
		}
		if e := q.DeleteEstimateLineItemsForEstimate(ctx, id); e != nil {
			return fmt.Errorf("clear items: %w", e)
		}
		return insertEstimateItems(ctx, q, id, items)
	})
	if err != nil {
		return nil, fmt.Errorf("update estimate: %w", err)
	}
	return r.Get(ctx, id)
}

// keepEstimateSnapshots preserves stored snapshots for any input left empty.
func keepEstimateSnapshots(in *EstimateInput, existing gen.GetEstimateRow) {
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

// estimateUpdateParams builds the update params, recomputing totals from items.
func estimateUpdateParams(in EstimateInput, items []LineItemInput, taxRate float64, id int64) gen.UpdateEstimateParams {
	t := computeTotals(items, taxRate)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpdateEstimateParams{
		ClientID:         nzInt(in.ClientID),
		Date:             in.Date,
		ValidUntil:       in.ValidUntil,
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
func (r *EstimatesRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "estimate", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if e := gen.New(tx).UpdateEstimateStatus(ctx, gen.UpdateEstimateStatusParams{
			Status: nz(status), UpdatedAt: now, ID: id,
		}); e != nil {
			return fmt.Errorf("update status: %w", e)
		}
		return nil
	})
}

// Delete removes an estimate (line items cascade) and writes one audit row.
func (r *EstimatesRepo) Delete(ctx context.Context, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "estimate", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).DeleteEstimate(ctx, id); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return nil
	})
}

// BulkDelete removes several estimates and writes one audit row. Empty is a no-op.
func (r *EstimatesRepo) BulkDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if e := q.DeleteEstimate(ctx, id); e != nil {
				return fmt.Errorf("delete %d: %w", id, e)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "estimate", EntityID: 0, Action: "bulk_delete",
			Changes: audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// BulkUpdateStatus sets the status of several estimates and writes one audit row.
func (r *EstimatesRepo) BulkUpdateStatus(ctx context.Context, ids []int64, status string) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		now := time.Now().UTC().Format(time.RFC3339)
		for _, id := range ids { // bounded by len(ids)
			if e := q.UpdateEstimateStatus(ctx, gen.UpdateEstimateStatusParams{
				Status: nz(status), UpdatedAt: now, ID: id,
			}); e != nil {
				return fmt.Errorf("status %d: %w", id, e)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "estimate", EntityID: 0, Action: "bulk_status",
			Changes: audit.Changes(map[string]any{"ids": ids, "status": status}),
		})
	})
}

// Duplicate creates a new draft estimate copying the source's client, tax rate,
// notes, currency, snapshots and line items, but resetting the date to today,
// clearing the valid-until and tax-rate id, and assigning a fresh number.
func (r *EstimatesRepo) Duplicate(ctx context.Context, id int64) (*Estimate, error) {
	src, err := r.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("duplicate estimate: %w", err)
	}
	if src == nil {
		return nil, errors.New("duplicate estimate: source not found")
	}
	in := EstimateInput{
		ClientID:         src.ClientID,
		Date:             time.Now().UTC().Format("2006-01-02"),
		ValidUntil:       "",
		TaxRate:          src.TaxRate,
		TaxRateID:        nil,
		Notes:            src.Notes,
		Status:           "draft",
		CurrencyCode:     src.CurrencyCode,
		BusinessSnapshot: src.BusinessSnapshot,
		ClientSnapshot:   src.ClientSnapshot,
		PayerSnapshot:    src.PayerSnapshot,
	}
	items := estimateLineItemsToInput(src.LineItems)

	var newID int64
	err = numbering.WithRetry(ctx, 10, func() error {
		return r.createTx(ctx, in, items, src.TaxRate, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("duplicate estimate: %w", err)
	}
	return r.Get(ctx, newID)
}

// estimateLineItemsToInput converts stored line items back into writable inputs.
func estimateLineItemsToInput(items []*EstimateLineItem) []LineItemInput {
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

// Convert turns an accepted estimate into a draft invoice (copying header and
// items, with valid_until becoming the invoice due date), links the estimate to
// the new invoice and flips it to 'converted'. Returns (nil, nil) when the
// estimate is missing, ErrNotAccepted unless status is 'accepted', and
// ErrAlreadyConverted when a linked invoice already exists.
func (r *EstimatesRepo) Convert(ctx context.Context, estimateID int64) (*ConvertResult, error) {
	est, err := r.Get(ctx, estimateID)
	if err != nil {
		return nil, fmt.Errorf("convert estimate: %w", err)
	}
	if est == nil {
		return nil, nil
	}
	if est.ConvertedInvoiceID != nil {
		return nil, ErrAlreadyConverted
	}
	if est.Status != "accepted" {
		return nil, ErrNotAccepted
	}

	var invID int64
	var invNum string
	err = numbering.WithRetry(ctx, 10, func() error {
		return r.convertTx(ctx, est, &invID, &invNum)
	})
	if err != nil {
		return nil, fmt.Errorf("convert estimate: %w", err)
	}
	return &ConvertResult{InvoiceID: invID, InvoiceNumber: invNum, EstimateNumber: est.EstimateNumber}, nil
}

// convertTx runs a single convert attempt inside one transaction: number the
// invoice, insert header + items, link the estimate, and audit.
func (r *EstimatesRepo) convertTx(ctx context.Context, est *Estimate, invID *int64, invNum *string) error {
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
	inv, err := q.CreateInvoice(ctx, buildInvoiceFromEstimate(est, num))
	if err != nil {
		return err
	}
	if err := copyEstimateItemsToInvoice(ctx, q, inv.ID, est.LineItems); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := q.SetEstimateConverted(ctx, gen.SetEstimateConvertedParams{
		ConvertedInvoiceID: nzInt(inv.ID), UpdatedAt: now, ID: est.ID,
	}); err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Entry{
		EntityType: "estimate", EntityID: est.ID, Action: "convert",
		Changes: audit.Changes(map[string]any{"invoiceId": inv.ID, "invoiceNumber": num}),
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*invID = inv.ID
	*invNum = num
	return nil
}

// buildInvoiceFromEstimate maps an estimate header onto invoice create params:
// valid_until becomes the due date, status resets to draft, payment terms to
// custom; totals and snapshots carry over verbatim.
func buildInvoiceFromEstimate(est *Estimate, num string) gen.CreateInvoiceParams {
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		Uuid:             uuid.NewString(),
		InvoiceNumber:    num,
		ClientID:         est.ClientID,
		Date:             est.Date,
		DueDate:          est.ValidUntil,
		PaymentTerms:     nz("custom"),
		Subtotal:         sql.NullFloat64{Float64: est.Subtotal, Valid: true},
		TaxRate:          sql.NullFloat64{Float64: est.TaxRate, Valid: true},
		TaxRateID:        nullID(est.TaxRateID),
		TaxAmount:        sql.NullFloat64{Float64: est.TaxAmount, Valid: true},
		Total:            sql.NullFloat64{Float64: est.Total, Valid: true},
		Notes:            nz(est.Notes),
		Status:           nz("draft"),
		CurrencyCode:     nz(orDefault(est.CurrencyCode, "USD")),
		BusinessSnapshot: nz(est.BusinessSnapshot),
		ClientSnapshot:   nz(est.ClientSnapshot),
		PayerSnapshot:    nz(est.PayerSnapshot),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// copyEstimateItemsToInvoice writes each estimate line item as an invoice line.
func copyEstimateItemsToInvoice(ctx context.Context, q *gen.Queries, invoiceID int64, items []*EstimateLineItem) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		_, err := q.CreateLineItem(ctx, gen.CreateLineItemParams{
			Uuid:          uuid.NewString(),
			InvoiceID:     invoiceID,
			Description:   it.Description,
			Quantity:      it.Quantity,
			Rate:          it.Rate,
			Amount:        it.Amount,
			Notes:         nz(it.Notes),
			SortOrder:     nzInt(it.SortOrder),
			CatalogItemID: nullID(it.CatalogItemID),
			RateTierID:    nullID(it.RateTierID),
		})
		if err != nil {
			return fmt.Errorf("copy estimate item %d: %w", i, err)
		}
	}
	return nil
}

// estimateFields is the shared, flat shape of every estimates join row.
type estimateFields struct {
	id                                  int64
	uuid, estimateNumber                string
	clientID                            sql.NullInt64
	date, validUntil                    string
	subtotal, taxRate                   sql.NullFloat64
	taxRateID                           sql.NullInt64
	taxAmount, total                    sql.NullFloat64
	notes, status, currencyCode         sql.NullString
	convertedInvoiceID                  sql.NullInt64
	businessSnap, clientSnap, payerSnap sql.NullString
	createdAt, updatedAt                string
	clientName                          sql.NullString
}

// toEstimateFromRow builds a domain Estimate (without line items) from the
// unwrapped join columns. LineItems defaults to a non-nil empty slice.
func toEstimateFromRow(f estimateFields) *Estimate {
	return &Estimate{
		ID:                 f.id,
		UUID:               f.uuid,
		EstimateNumber:     f.estimateNumber,
		ClientID:           f.clientID.Int64,
		ClientName:         f.clientName.String,
		Date:               f.date,
		ValidUntil:         f.validUntil,
		Subtotal:           f.subtotal.Float64,
		TaxRate:            f.taxRate.Float64,
		TaxRateID:          ptrID(f.taxRateID),
		TaxAmount:          f.taxAmount.Float64,
		Total:              f.total.Float64,
		Notes:              f.notes.String,
		Status:             f.status.String,
		CurrencyCode:       f.currencyCode.String,
		ConvertedInvoiceID: ptrID(f.convertedInvoiceID),
		BusinessSnapshot:   f.businessSnap.String,
		ClientSnapshot:     f.clientSnap.String,
		PayerSnapshot:      f.payerSnap.String,
		CreatedAt:          f.createdAt,
		UpdatedAt:          f.updatedAt,
		LineItems:          []*EstimateLineItem{},
	}
}

func estimateFieldsFromGet(r gen.GetEstimateRow) estimateFields {
	return estimateFields{
		id: r.ID, uuid: r.Uuid, estimateNumber: r.EstimateNumber, clientID: r.ClientID,
		date: r.Date, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, taxRate: r.TaxRate, taxRateID: r.TaxRateID,
		taxAmount: r.TaxAmount, total: r.Total,
		notes: r.Notes, status: r.Status, currencyCode: r.CurrencyCode,
		convertedInvoiceID: r.ConvertedInvoiceID,
		businessSnap:       r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

func estimateFieldsFromList(r gen.ListEstimatesRow) estimateFields {
	return estimateFields{
		id: r.ID, uuid: r.Uuid, estimateNumber: r.EstimateNumber, clientID: r.ClientID,
		date: r.Date, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, taxRate: r.TaxRate, taxRateID: r.TaxRateID,
		taxAmount: r.TaxAmount, total: r.Total,
		notes: r.Notes, status: r.Status, currencyCode: r.CurrencyCode,
		convertedInvoiceID: r.ConvertedInvoiceID,
		businessSnap:       r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

func estimateFieldsFromStatus(r gen.ListEstimatesByStatusRow) estimateFields {
	return estimateFields{
		id: r.ID, uuid: r.Uuid, estimateNumber: r.EstimateNumber, clientID: r.ClientID,
		date: r.Date, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, taxRate: r.TaxRate, taxRateID: r.TaxRateID,
		taxAmount: r.TaxAmount, total: r.Total,
		notes: r.Notes, status: r.Status, currencyCode: r.CurrencyCode,
		convertedInvoiceID: r.ConvertedInvoiceID,
		businessSnap:       r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

func estimateFieldsFromClient(r gen.ListClientEstimatesRow) estimateFields {
	return estimateFields{
		id: r.ID, uuid: r.Uuid, estimateNumber: r.EstimateNumber, clientID: r.ClientID,
		date: r.Date, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, taxRate: r.TaxRate, taxRateID: r.TaxRateID,
		taxAmount: r.TaxAmount, total: r.Total,
		notes: r.Notes, status: r.Status, currencyCode: r.CurrencyCode,
		convertedInvoiceID: r.ConvertedInvoiceID,
		businessSnap:       r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

// mapEstimateLineItems maps generated line item rows to domain line items.
func mapEstimateLineItems(rows []gen.EstimateLineItem) []*EstimateLineItem {
	out := make([]*EstimateLineItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toEstimateLineItem(rows[i]))
	}
	return out
}

// toEstimateLineItem maps one generated line item to the domain shape.
func toEstimateLineItem(row gen.EstimateLineItem) *EstimateLineItem {
	return &EstimateLineItem{
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
