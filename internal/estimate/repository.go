package estimate

// NOTE (J4): rewritten to the estimate domain (spec §4.2), parallel to the
// invoice rewrite. Same design decisions apply: `tax` is supplied on the header
// (computed upstream by J10); this repo only sums line totals and rounds at each
// boundary; no price-cap / plan-window validation here (J10). Per-tenant
// numbering is allocated inline via gen.MaxEstimateNumberLike inside the tx.

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

// Estimate is the domain view of an estimate with its resolved client name
// and embedded line items. Mirrors Invoice with estimate-specific deltas:
// valid_until replaces due_date, and an optional converted_invoice_id records
// the invoice produced by Convert.
type Estimate struct {
	ID                   string              `json:"id"` // public identifier (estimate uuid)
	Number               string              `json:"number"`
	ClientID             *string             `json:"-"`        // internal FK; the public ref is clientId (uuid)
	ClientUUID           string              `json:"clientId"` // client uuid
	ClientName           string              `json:"clientName"`
	PayerID              *string             `json:"-"`       // internal FK; the public ref is payerId (uuid)
	PayerUUID            *string             `json:"payerId"` // payer uuid (nil when none)
	Status               string              `json:"status"`
	IssueDate            string              `json:"issueDate"`
	ValidUntil           string              `json:"validUntil"`
	Subtotal             float64             `json:"subtotal"`
	Tax                  float64             `json:"tax"`
	Total                float64             `json:"total"`
	Notes                string              `json:"notes"`
	ConvertedInvoiceID   *string             `json:"-"`                  // internal FK; the public ref is convertedInvoiceId (the produced invoice's uuid)
	ConvertedInvoiceUUID *string             `json:"convertedInvoiceId"` // produced invoice uuid (nil until converted)
	BusinessSnapshot     string              `json:"businessSnapshot"`
	ClientSnapshot       string              `json:"clientSnapshot"`
	PayerSnapshot        string              `json:"payerSnapshot"`
	CreatedAt            string              `json:"createdAt"`
	UpdatedAt            string              `json:"updatedAt"`
	LineItems            []*billing.LineItem `json:"lineItems"`
}

// EstimateInput is the writable subset of an estimate header.
type EstimateInput struct {
	ClientID         string  `json:"clientId"`
	PayerID          *string `json:"payerId"`
	Status           string  `json:"status"`
	IssueDate        string  `json:"issueDate"`
	ValidUntil       string  `json:"validUntil"`
	Tax              float64 `json:"tax"`
	Notes            string  `json:"notes"`
	BusinessSnapshot string  `json:"businessSnapshot"`
	ClientSnapshot   string  `json:"clientSnapshot"`
	PayerSnapshot    string  `json:"payerSnapshot"`
}

// ConvertResult identifies the invoice produced by Convert. The public
// identifier is the new invoice's uuid, serialized as "id"; the internal row id
// is kept out of the JSON (the service reads it to broadcast the invoice "create" event).
type ConvertResult struct {
	InvoiceID      string `json:"-"`  // internal invoice id (uuid)
	InvoiceUUID    string `json:"id"` // public identifier (new invoice uuid)
	InvoiceNumber  string `json:"invoiceNumber"`
	EstimateNumber string `json:"estimateNumber"`
}

// EstimatesRepo reads and writes the estimates + estimate_line_items tables
// (tenant-scoped).
type EstimatesRepo struct {
	db   db.Executor
	snap *billing.SnapshotBuilder
}

// NewEstimates constructs a repository. A nil db is a programmer error.
func NewEstimates(db db.Executor) *EstimatesRepo {
	if db == nil {
		panic("estimate: NewEstimates requires a non-nil *sql.DB")
	}
	return &EstimatesRepo{db: db, snap: billing.NewSnapshotBuilder(db)}
}

// fillSnapshots fills any empty snapshot field on in with a default built from
// the business profile, client and payer.
func (r *EstimatesRepo) fillSnapshots(ctx context.Context, tenantID string, in *EstimateInput) {
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

// Create inserts an estimate plus its line items inside one numbering-retried
// transaction, audits the create, and re-reads the row. ClientID and at
// least one line item are required.
func (r *EstimatesRepo) Create(ctx context.Context, tenantID string, in EstimateInput, items []billing.LineItemInput) (*Estimate, error) {
	if tenantID == "" {
		return nil, errors.New("create estimate: tenant id required")
	}
	if in.ClientID == "" {
		return nil, errors.New("create estimate: client is required")
	}
	if len(items) == 0 {
		return nil, errors.New("create estimate: at least one line item is required")
	}
	r.fillSnapshots(ctx, tenantID, &in)

	var newID string
	err := numbering.WithRetry(ctx, 10, func() error {
		return r.createTx(ctx, tenantID, in, items, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("create estimate: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// createTx runs a single create attempt in one transaction.
func (r *EstimatesRepo) createTx(ctx context.Context, tenantID string, in EstimateInput, items []billing.LineItemInput, newID *string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	num, err := nextEstimateNumber(ctx, q, tenantID)
	if err != nil {
		return err
	}
	est, err := q.CreateEstimate(ctx, createEstimateParams(tenantID, in, items, num))
	if err != nil {
		return err
	}
	if err := insertEstimateItems(ctx, q, tenantID, est.ID, items); err != nil {
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

// nextEstimateNumber allocates the next per-tenant estimate number ("EST-NNNN").
func nextEstimateNumber(ctx context.Context, q *gen.Queries, tenantID string) (string, error) {
	const prefix = "EST-"
	max, err := q.MaxEstimateNumberLike(ctx, gen.MaxEstimateNumberLikeParams{
		PrefixLen: int64(len(prefix)),
		TenantID:  tenantID,
		Pattern:   prefix + "%",
	})
	if err != nil {
		return "", fmt.Errorf("next estimate number: %w", err)
	}
	return numbering.Format(prefix, max), nil
}

// createEstimateParams builds the insert params, applying defaults and totals.
func createEstimateParams(tenantID string, in EstimateInput, items []billing.LineItemInput, num string) gen.CreateEstimateParams {
	t := billing.ComputeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateEstimateParams{
		ID:                 ids.New(),
		TenantID:           tenantID,
		Number:             num,
		ClientID:           db.NullStr(&in.ClientID),
		PayerID:            db.NullStr(in.PayerID),
		Status:             orDefault(in.Status, "draft"),
		IssueDate:          in.IssueDate,
		ValidUntil:         in.ValidUntil,
		Subtotal:           t.Subtotal,
		Tax:                t.Tax,
		Total:              t.Total,
		Notes:              db.NzMaybe(in.Notes),
		ConvertedInvoiceID: sql.NullString{},
		BusinessSnapshot:   db.NzMaybe(in.BusinessSnapshot),
		ClientSnapshot:     db.NzMaybe(in.ClientSnapshot),
		PayerSnapshot:      db.NzMaybe(in.PayerSnapshot),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// insertEstimateItems writes each line item with its computed total.
func insertEstimateItems(ctx context.Context, q *gen.Queries, tenantID, estimateID string, items []billing.LineItemInput) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		catalogueItemID, err := billing.ResolveCatalogueItemID(ctx, q, tenantID, it.CatalogueItemID)
		if err != nil {
			return fmt.Errorf("insert estimate line item %d: %w", i, err)
		}
		_, err = q.CreateEstimateLineItem(ctx, gen.CreateEstimateLineItemParams{
			ID:              ids.New(),
			TenantID:        tenantID,
			EstimateID:      estimateID,
			CatalogueItemID: catalogueItemID,
			Code:            db.NzMaybe(it.Code),
			Description:     it.Description,
			ServiceDate:     db.NzMaybe(it.ServiceDate),
			Unit:            db.NzMaybe(it.Unit),
			Quantity:        it.Quantity,
			UnitPrice:       it.UnitPrice,
			Taxable:         db.B2i(it.Taxable),
			LineTotal:       billing.Round2(it.Quantity * it.UnitPrice),
			SortOrder:       sql.NullInt64{Int64: it.SortOrder, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("insert estimate line item %d: %w", i, err)
		}
	}
	return nil
}

// Get returns the estimate (with client name and line items) by row id (uuid), or
// (nil, nil) when absent. Internal read used by the convert/duplicate paths and
// the id-keyed service methods.
func (r *EstimatesRepo) Get(ctx context.Context, tenantID, id string) (*Estimate, error) {
	q := gen.New(r.db)
	row, err := q.GetEstimateByID(ctx, gen.GetEstimateByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get estimate: %w", err)
	}
	est := toEstimateFromRow(estimateFieldsFromGetByID(row))
	return r.withLineItems(ctx, q, tenantID, est)
}

// GetByUUID returns the estimate (with line items) addressed by its uuid, or
// (nil, nil) when no estimate matches the uuid for the tenant. Public HTTP read.
func (r *EstimatesRepo) GetByUUID(ctx context.Context, tenantID string, estimateUUID string) (*Estimate, error) {
	q := gen.New(r.db)
	row, err := q.GetEstimate(ctx, gen.GetEstimateParams{TenantID: tenantID, ID: estimateUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get estimate by uuid: %w", err)
	}
	est := toEstimateFromRow(estimateFieldsFromGet(row))
	return r.withLineItems(ctx, q, tenantID, est)
}

// withLineItems loads an estimate's line items (keyed by the estimate's row id (uuid)).
func (r *EstimatesRepo) withLineItems(ctx context.Context, q *gen.Queries, tenantID string, est *Estimate) (*Estimate, error) {
	rows, err := q.ListEstimateLineItems(ctx, gen.ListEstimateLineItemsParams{TenantID: tenantID, EstimateID: est.ID})
	if err != nil {
		return nil, fmt.Errorf("list estimate line items: %w", err)
	}
	est.LineItems = mapEstimateLineItems(rows)
	return est, nil
}

// Update rewrites the header (recomputing totals) and replaces all line items,
// atomically with one audit row. Empty snapshot inputs keep the existing stored
// snapshots. Returns (nil, nil) when the estimate does not exist.
func (r *EstimatesRepo) Update(ctx context.Context, tenantID, id string, in EstimateInput, items []billing.LineItemInput) (*Estimate, error) {
	if in.ClientID == "" {
		return nil, errors.New("update estimate: client is required")
	}
	if len(items) == 0 {
		return nil, errors.New("update estimate: at least one line item is required")
	}
	existing, err := gen.New(r.db).GetEstimateByID(ctx, gen.GetEstimateByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update estimate: load existing: %w", err)
	}
	keepEstimateSnapshots(&in, existing)

	err = audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "estimate", EntityID: id, Action: "update",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		if _, e := q.UpdateEstimate(ctx, updateEstimateParams(tenantID, in, items, existing.Number, id)); e != nil {
			return fmt.Errorf("update: %w", e)
		}
		if e := q.DeleteEstimateLineItemsForEstimate(ctx, gen.DeleteEstimateLineItemsForEstimateParams{TenantID: tenantID, EstimateID: id}); e != nil {
			return fmt.Errorf("clear items: %w", e)
		}
		return insertEstimateItems(ctx, q, tenantID, id, items)
	})
	if err != nil {
		return nil, fmt.Errorf("update estimate: %w", err)
	}
	return r.Get(ctx, tenantID, id)
}

// keepEstimateSnapshots preserves stored snapshots for any input left empty.
func keepEstimateSnapshots(in *EstimateInput, existing gen.GetEstimateByIDRow) {
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

// updateEstimateParams builds the update params; the number is immutable.
func updateEstimateParams(tenantID string, in EstimateInput, items []billing.LineItemInput, number string, id string) gen.UpdateEstimateParams {
	t := billing.ComputeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpdateEstimateParams{
		Number:           number,
		ClientID:         db.NullStr(&in.ClientID),
		PayerID:          db.NullStr(in.PayerID),
		Status:           orDefault(in.Status, "draft"),
		IssueDate:        in.IssueDate,
		ValidUntil:       in.ValidUntil,
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

// Delete removes an estimate (line items cascade) and writes one audit row.
func (r *EstimatesRepo) Delete(ctx context.Context, tenantID, id string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "estimate", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).DeleteEstimate(ctx, gen.DeleteEstimateParams{TenantID: tenantID, ID: id}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return nil
	})
}
