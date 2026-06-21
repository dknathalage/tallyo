package estimate

// NOTE (J4): rewritten to the NDIS estimate domain (spec §4.2), parallel to the
// invoice rewrite. Same design decisions apply: `tax` is supplied on the header
// (computed upstream by J10); this repo only sums line totals and rounds at each
// boundary; no NDIS price-cap / plan-window validation here (J10). Per-tenant
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
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/numbering"
	"github.com/google/uuid"
)

// estimateListSelect mirrors the ListEstimates sqlc query body up to the WHERE.
// Keep in sync with internal/db/queries/estimates.sql. tenant_id is the only ?.
const estimateListSelect = `SELECT e.*, p.name AS participant_name
FROM estimates e
LEFT JOIN participants p ON e.participant_id = p.id AND p.tenant_id = e.tenant_id
WHERE e.tenant_id = ?`

// EstimateCols is the listquery allowlist for estimates. Keys match the JSON
// field names so one column key drives filter, sort and display.
var EstimateCols = listquery.Spec{
	"number":          {Col: "e.number", Filter: listquery.Text},
	"participantName": {Col: "p.name", Filter: listquery.Text},
	"status":          {Col: "e.status", Filter: listquery.Enum},
	"issueDate":       {Col: "e.issue_date", Filter: listquery.Date},
	"total":           {Col: "e.total", Filter: listquery.Number},
}

// ErrNotAccepted is returned when converting an estimate that is not in the
// 'accepted' state.
var ErrNotAccepted = errors.New("only accepted estimates can be converted")

// ErrAlreadyConverted is returned when converting an estimate that already has
// a linked invoice.
var ErrAlreadyConverted = errors.New("estimate already converted")

// Estimate is the domain view of an estimate with its resolved participant name
// and embedded line items. Mirrors Invoice with estimate-specific deltas:
// valid_until replaces due_date, and an optional converted_invoice_id records
// the invoice produced by Convert.
type Estimate struct {
	ID                 int64               `json:"id"`
	UUID               string              `json:"uuid"`
	Number             string              `json:"number"`
	ParticipantID      *int64              `json:"participantId"`
	ParticipantName    string              `json:"participantName"`
	PlanManagerID      *int64              `json:"planManagerId"`
	Status             string              `json:"status"`
	IssueDate          string              `json:"issueDate"`
	ValidUntil         string              `json:"validUntil"`
	Subtotal           float64             `json:"subtotal"`
	Tax                float64             `json:"tax"`
	Total              float64             `json:"total"`
	Notes              string              `json:"notes"`
	ConvertedInvoiceID *int64              `json:"convertedInvoiceId"`
	BusinessSnapshot   string              `json:"businessSnapshot"`
	ClientSnapshot     string              `json:"participantSnapshot"`
	PayerSnapshot      string              `json:"planManagerSnapshot"`
	CreatedAt          string              `json:"createdAt"`
	UpdatedAt          string              `json:"updatedAt"`
	LineItems          []*billing.LineItem `json:"lineItems"`
}

// EstimateInput is the writable subset of an estimate header.
type EstimateInput struct {
	ParticipantID    int64   `json:"participantId"`
	PlanManagerID    *int64  `json:"planManagerId"`
	Status           string  `json:"status"`
	IssueDate        string  `json:"issueDate"`
	ValidUntil       string  `json:"validUntil"`
	Tax              float64 `json:"tax"`
	Notes            string  `json:"notes"`
	BusinessSnapshot string  `json:"businessSnapshot"`
	ClientSnapshot   string  `json:"participantSnapshot"`
	PayerSnapshot    string  `json:"planManagerSnapshot"`
}

// ConvertResult identifies the invoice produced by Convert.
type ConvertResult struct {
	InvoiceID      int64  `json:"invoiceId"`
	InvoiceNumber  string `json:"invoiceNumber"`
	EstimateNumber string `json:"estimateNumber"`
}

// EstimatesRepo reads and writes the estimates + estimate_line_items tables
// (tenant-scoped).
type EstimatesRepo struct {
	db   *sql.DB
	snap *billing.SnapshotBuilder
}

// NewEstimates constructs a repository. A nil db is a programmer error.
func NewEstimates(db *sql.DB) *EstimatesRepo {
	if db == nil {
		panic("estimate: NewEstimates requires a non-nil *sql.DB")
	}
	return &EstimatesRepo{db: db, snap: billing.NewSnapshotBuilder(db)}
}

// fillSnapshots fills any empty snapshot field on in with a default built from
// the business profile, participant and plan manager.
func (r *EstimatesRepo) fillSnapshots(ctx context.Context, tenantID int64, in *EstimateInput) {
	if in.BusinessSnapshot == "" {
		in.BusinessSnapshot = r.snap.Business(ctx, tenantID)
	}
	if in.ClientSnapshot == "" {
		in.ClientSnapshot = r.snap.Participant(ctx, tenantID, in.ParticipantID)
	}
	if in.PayerSnapshot == "" {
		in.PayerSnapshot = r.snap.PlanManager(ctx, tenantID, in.PlanManagerID)
	}
}

// Create inserts an estimate plus its line items inside one numbering-retried
// transaction, audits the create, and re-reads the row. ParticipantID and at
// least one line item are required.
func (r *EstimatesRepo) Create(ctx context.Context, tenantID int64, in EstimateInput, items []billing.LineItemInput) (*Estimate, error) {
	if tenantID == 0 {
		return nil, errors.New("create estimate: tenant id required")
	}
	if in.ParticipantID == 0 {
		return nil, errors.New("create estimate: participant is required")
	}
	if len(items) == 0 {
		return nil, errors.New("create estimate: at least one line item is required")
	}
	r.fillSnapshots(ctx, tenantID, &in)

	var newID int64
	err := numbering.WithRetry(ctx, 10, func() error {
		return r.createTx(ctx, tenantID, in, items, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("create estimate: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// createTx runs a single create attempt in one transaction.
func (r *EstimatesRepo) createTx(ctx context.Context, tenantID int64, in EstimateInput, items []billing.LineItemInput, newID *int64) error {
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
func nextEstimateNumber(ctx context.Context, q *gen.Queries, tenantID int64) (string, error) {
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
func createEstimateParams(tenantID int64, in EstimateInput, items []billing.LineItemInput, num string) gen.CreateEstimateParams {
	t := billing.ComputeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateEstimateParams{
		Uuid:               uuid.NewString(),
		TenantID:           tenantID,
		Number:             num,
		ParticipantID:      db.NullID(&in.ParticipantID),
		PlanManagerID:      db.NullID(in.PlanManagerID),
		Status:             orDefault(in.Status, "draft"),
		IssueDate:          in.IssueDate,
		ValidUntil:         in.ValidUntil,
		Subtotal:           t.Subtotal,
		Tax:                t.Tax,
		Total:              t.Total,
		Notes:              db.NzMaybe(in.Notes),
		ConvertedInvoiceID: sql.NullInt64{},
		BusinessSnapshot:   db.NzMaybe(in.BusinessSnapshot),
		ClientSnapshot:     db.NzMaybe(in.ClientSnapshot),
		PayerSnapshot:      db.NzMaybe(in.PayerSnapshot),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// insertEstimateItems writes each line item with its computed total.
func insertEstimateItems(ctx context.Context, q *gen.Queries, tenantID, estimateID int64, items []billing.LineItemInput) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		_, err := q.CreateEstimateLineItem(ctx, gen.CreateEstimateLineItemParams{
			Uuid:             uuid.NewString(),
			TenantID:         tenantID,
			EstimateID:       estimateID,
			SupportItemID:    db.NullStr(it.SupportItemID),
			CustomItemID:     db.NullID(it.CustomItemID),
			CatalogVersionID: db.NullStr(it.CatalogVersionID),
			Code:             db.NzMaybe(it.Code),
			Description:      it.Description,
			ServiceDate:      db.NzMaybe(it.ServiceDate),
			Unit:             db.NzMaybe(it.Unit),
			Quantity:         it.Quantity,
			UnitPrice:        it.UnitPrice,
			GstFree:          db.B2i(it.GstFree),
			LineTotal:        billing.Round2(it.Quantity * it.UnitPrice),
			SortOrder:        sql.NullInt64{Int64: it.SortOrder, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("insert estimate line item %d: %w", i, err)
		}
	}
	return nil
}

// Get returns the estimate (with participant name and line items), or (nil, nil)
// when absent.
func (r *EstimatesRepo) Get(ctx context.Context, tenantID, id int64) (*Estimate, error) {
	q := gen.New(r.db)
	row, err := q.GetEstimate(ctx, gen.GetEstimateParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get estimate: %w", err)
	}
	est := toEstimateFromRow(estimateFieldsFromGet(row))
	rows, err := q.ListEstimateLineItems(ctx, gen.ListEstimateLineItemsParams{TenantID: tenantID, EstimateID: id})
	if err != nil {
		return nil, fmt.Errorf("list estimate line items: %w", err)
	}
	est.LineItems = mapEstimateLineItems(rows)
	return est, nil
}

// List returns every estimate for the tenant (header only), newest first.
func (r *EstimatesRepo) List(ctx context.Context, tenantID int64) ([]*Estimate, error) {
	rows, err := gen.New(r.db).ListEstimates(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list estimates: %w", err)
	}
	out := make([]*Estimate, 0, len(rows))
	for i := range rows {
		out = append(out, toEstimateFromRow(estimateFieldsFromList(rows[i])))
	}
	return out, nil
}

// Query returns one page of estimates plus the total row count for the filter
// (ignoring pagination). The clause is built by listquery from an allowlisted
// spec, so its Where/Order fragments are injection-safe. Default order matches
// ListEstimates (newest first) when no sort is supplied.
func (r *EstimatesRepo) Query(ctx context.Context, tenantID int64, c listquery.Clause) ([]*Estimate, int64, error) {
	if tenantID == 0 {
		return nil, 0, errors.New("query estimates: tenant id required")
	}
	var total int64
	countSQL := "SELECT count(*) FROM (" + estimateListSelect + c.Where + ")"
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count estimates: %w", err)
	}
	order := c.Order
	if order == "" {
		order = " ORDER BY e.created_at DESC"
	}
	sqlText := estimateListSelect + c.Where + order + c.Limit
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query estimates: %w", err)
	}
	defer rows.Close()
	out := make([]*Estimate, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var f estimateFields
		var tenant int64
		if err := rows.Scan(&f.id, &f.uuid, &tenant, &f.number, &f.participantID,
			&f.planManagerID, &f.status, &f.issueDate, &f.validUntil, &f.subtotal,
			&f.tax, &f.total, &f.notes, &f.convertedInvoiceID, &f.businessSnap,
			&f.clientSnap, &f.payerSnap, &f.createdAt, &f.updatedAt, &f.participantName); err != nil {
			return nil, 0, fmt.Errorf("scan estimate: %w", err)
		}
		out = append(out, toEstimateFromRow(f))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query estimates: %w", err)
	}
	return out, total, nil
}

// ListByStatus returns the tenant's estimates with the given status.
func (r *EstimatesRepo) ListByStatus(ctx context.Context, tenantID int64, status string) ([]*Estimate, error) {
	rows, err := gen.New(r.db).ListEstimatesByStatus(ctx, gen.ListEstimatesByStatusParams{
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return nil, fmt.Errorf("list estimates by status: %w", err)
	}
	out := make([]*Estimate, 0, len(rows))
	for i := range rows {
		out = append(out, toEstimateFromRow(estimateFieldsFromStatus(rows[i])))
	}
	return out, nil
}

// ListParticipantEstimates returns one participant's estimates (header only).
func (r *EstimatesRepo) ListParticipantEstimates(ctx context.Context, tenantID, participantID int64) ([]*Estimate, error) {
	rows, err := gen.New(r.db).ListParticipantEstimates(ctx, gen.ListParticipantEstimatesParams{
		TenantID:      tenantID,
		ParticipantID: sql.NullInt64{Int64: participantID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list participant estimates: %w", err)
	}
	out := make([]*Estimate, 0, len(rows))
	for i := range rows {
		out = append(out, toEstimateFromRow(estimateFieldsFromParticipant(rows[i])))
	}
	return out, nil
}

// Update rewrites the header (recomputing totals) and replaces all line items,
// atomically with one audit row. Empty snapshot inputs keep the existing stored
// snapshots. Returns (nil, nil) when the estimate does not exist.
func (r *EstimatesRepo) Update(ctx context.Context, tenantID, id int64, in EstimateInput, items []billing.LineItemInput) (*Estimate, error) {
	if in.ParticipantID == 0 {
		return nil, errors.New("update estimate: participant is required")
	}
	if len(items) == 0 {
		return nil, errors.New("update estimate: at least one line item is required")
	}
	existing, err := gen.New(r.db).GetEstimate(ctx, gen.GetEstimateParams{TenantID: tenantID, ID: id})
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

// updateEstimateParams builds the update params; the number is immutable.
func updateEstimateParams(tenantID int64, in EstimateInput, items []billing.LineItemInput, number string, id int64) gen.UpdateEstimateParams {
	t := billing.ComputeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpdateEstimateParams{
		Number:           number,
		ParticipantID:    db.NullID(&in.ParticipantID),
		PlanManagerID:    db.NullID(in.PlanManagerID),
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

// UpdateStatus sets just the status column, atomically with one audit row.
func (r *EstimatesRepo) UpdateStatus(ctx context.Context, tenantID, id int64, status string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "estimate", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if e := gen.New(tx).UpdateEstimateStatus(ctx, gen.UpdateEstimateStatusParams{
			Status: status, UpdatedAt: now, TenantID: tenantID, ID: id,
		}); e != nil {
			return fmt.Errorf("update status: %w", e)
		}
		return nil
	})
}

// Delete removes an estimate (line items cascade) and writes one audit row.
func (r *EstimatesRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "estimate", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).DeleteEstimate(ctx, gen.DeleteEstimateParams{TenantID: tenantID, ID: id}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return nil
	})
}

// BulkDelete removes several estimates and writes one audit row. Empty is a no-op.
func (r *EstimatesRepo) BulkDelete(ctx context.Context, tenantID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if e := q.DeleteEstimate(ctx, gen.DeleteEstimateParams{TenantID: tenantID, ID: id}); e != nil {
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
func (r *EstimatesRepo) BulkUpdateStatus(ctx context.Context, tenantID int64, ids []int64, status string) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		now := time.Now().UTC().Format(time.RFC3339)
		for _, id := range ids { // bounded by len(ids)
			if e := q.UpdateEstimateStatus(ctx, gen.UpdateEstimateStatusParams{
				Status: status, UpdatedAt: now, TenantID: tenantID, ID: id,
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

// Duplicate creates a new draft estimate copying the source's participant, plan
// manager, tax, notes, snapshots and line items, resetting the date to today,
// clearing valid-until, and assigning a fresh number.
func (r *EstimatesRepo) Duplicate(ctx context.Context, tenantID, id int64) (*Estimate, error) {
	src, err := r.Get(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("duplicate estimate: %w", err)
	}
	if src == nil {
		return nil, errors.New("duplicate estimate: source not found")
	}
	var participantID int64
	if src.ParticipantID != nil {
		participantID = *src.ParticipantID
	}
	in := EstimateInput{
		ParticipantID:    participantID,
		PlanManagerID:    src.PlanManagerID,
		Status:           "draft",
		IssueDate:        time.Now().UTC().Format("2006-01-02"),
		ValidUntil:       "",
		Tax:              src.Tax,
		Notes:            src.Notes,
		BusinessSnapshot: src.BusinessSnapshot,
		ClientSnapshot:   src.ClientSnapshot,
		PayerSnapshot:    src.PayerSnapshot,
	}
	items := lineItemsToInput(src.LineItems)

	var newID int64
	err = numbering.WithRetry(ctx, 10, func() error {
		return r.createTx(ctx, tenantID, in, items, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("duplicate estimate: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// Convert turns an accepted estimate into a draft invoice (copying header and
// items, with valid_until becoming the invoice due date), links the estimate to
// the new invoice and flips it to 'converted'. Returns (nil, nil) when the
// estimate is missing, ErrNotAccepted unless status is 'accepted', and
// ErrAlreadyConverted when a linked invoice already exists.
func (r *EstimatesRepo) Convert(ctx context.Context, tenantID, estimateID int64) (*ConvertResult, error) {
	est, err := r.Get(ctx, tenantID, estimateID)
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
		return r.convertTx(ctx, tenantID, est, &invID, &invNum)
	})
	if err != nil {
		return nil, fmt.Errorf("convert estimate: %w", err)
	}
	return &ConvertResult{InvoiceID: invID, InvoiceNumber: invNum, EstimateNumber: est.Number}, nil
}

// convertTx runs a single convert attempt inside one transaction.
func (r *EstimatesRepo) convertTx(ctx context.Context, tenantID int64, est *Estimate, invID *int64, invNum *string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	num, err := invoice.NextInvoiceNumber(ctx, q, tenantID)
	if err != nil {
		return err
	}
	inv, err := q.CreateInvoice(ctx, buildInvoiceFromEstimate(tenantID, est, num))
	if err != nil {
		return err
	}
	if err := copyEstimateItemsToInvoice(ctx, q, tenantID, inv.ID, est.LineItems); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := q.SetEstimateConverted(ctx, gen.SetEstimateConvertedParams{
		ConvertedInvoiceID: sql.NullInt64{Int64: inv.ID, Valid: true}, UpdatedAt: now, TenantID: tenantID, ID: est.ID,
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

// buildInvoiceFromEstimate maps an estimate header onto invoice create params.
func buildInvoiceFromEstimate(tenantID int64, est *Estimate, num string) gen.CreateInvoiceParams {
	var participantID int64
	if est.ParticipantID != nil {
		participantID = *est.ParticipantID
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		Uuid:             uuid.NewString(),
		TenantID:         tenantID,
		Number:           num,
		ParticipantID:    participantID,
		PlanManagerID:    db.NullID(est.PlanManagerID),
		Status:           "draft",
		IssueDate:        est.IssueDate,
		DueDate:          est.ValidUntil,
		Subtotal:         est.Subtotal,
		Tax:              est.Tax,
		Total:            est.Total,
		Notes:            db.NzMaybe(est.Notes),
		BusinessSnapshot: db.NzMaybe(est.BusinessSnapshot),
		ClientSnapshot:   db.NzMaybe(est.ClientSnapshot),
		PayerSnapshot:    db.NzMaybe(est.PayerSnapshot),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// copyEstimateItemsToInvoice writes each estimate line item as an invoice line.
func copyEstimateItemsToInvoice(ctx context.Context, q *gen.Queries, tenantID, invoiceID int64, items []*billing.LineItem) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		_, err := q.CreateLineItem(ctx, gen.CreateLineItemParams{
			Uuid:             uuid.NewString(),
			TenantID:         tenantID,
			ShiftID:          sql.NullInt64{}, // estimate-converted lines are not shift items
			InvoiceID:        sql.NullInt64{Int64: invoiceID, Valid: true},
			SupportItemID:    db.NullStr(it.SupportItemID),
			CustomItemID:     db.NullID(it.CustomItemID),
			CatalogVersionID: db.NullStr(it.CatalogVersionID),
			Code:             db.NzMaybe(it.Code),
			Description:      it.Description,
			ServiceDate:      db.NzMaybe(it.ServiceDate),
			Unit:             db.NzMaybe(it.Unit),
			Quantity:         it.Quantity,
			UnitPrice:        it.UnitPrice,
			GstFree:          db.B2i(it.GstFree),
			LineTotal:        it.LineTotal,
			SortOrder:        sql.NullInt64{Int64: it.SortOrder, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("copy estimate item %d: %w", i, err)
		}
	}
	return nil
}

// lineItemsToInput converts stored line items back into writable inputs.
func lineItemsToInput(items []*billing.LineItem) []billing.LineItemInput {
	out := make([]billing.LineItemInput, 0, len(items))
	for i := range items { // bounded by len(items)
		it := items[i]
		out = append(out, billing.LineItemInput{
			SupportItemID:    it.SupportItemID,
			CustomItemID:     it.CustomItemID,
			CatalogVersionID: it.CatalogVersionID,
			Code:             it.Code,
			Description:      it.Description,
			ServiceDate:      it.ServiceDate,
			Unit:             it.Unit,
			Quantity:         it.Quantity,
			UnitPrice:        it.UnitPrice,
			GstFree:          it.GstFree,
			SortOrder:        it.SortOrder,
		})
	}
	return out
}

// estimateFields is the shared, flat shape of every estimates join row.
type estimateFields struct {
	id                                  int64
	uuid, number                        string
	participantID                       sql.NullInt64
	planManagerID                       sql.NullInt64
	status, issueDate, validUntil       string
	subtotal, tax, total                float64
	notes                               sql.NullString
	convertedInvoiceID                  sql.NullInt64
	businessSnap, clientSnap, payerSnap sql.NullString
	createdAt, updatedAt                string
	participantName                     sql.NullString
}

// toEstimateFromRow builds a domain Estimate (without line items).
func toEstimateFromRow(f estimateFields) *Estimate {
	return &Estimate{
		ID:                 f.id,
		UUID:               f.uuid,
		Number:             f.number,
		ParticipantID:      db.PtrID(f.participantID),
		ParticipantName:    f.participantName.String,
		PlanManagerID:      db.PtrID(f.planManagerID),
		Status:             f.status,
		IssueDate:          f.issueDate,
		ValidUntil:         f.validUntil,
		Subtotal:           f.subtotal,
		Tax:                f.tax,
		Total:              f.total,
		Notes:              f.notes.String,
		ConvertedInvoiceID: db.PtrID(f.convertedInvoiceID),
		BusinessSnapshot:   f.businessSnap.String,
		ClientSnapshot:     f.clientSnap.String,
		PayerSnapshot:      f.payerSnap.String,
		CreatedAt:          f.createdAt,
		UpdatedAt:          f.updatedAt,
		LineItems:          []*billing.LineItem{},
	}
}

func estimateFieldsFromGet(r gen.GetEstimateRow) estimateFields {
	return estimateFields{
		id: r.ID, uuid: r.Uuid, number: r.Number, participantID: r.ParticipantID,
		planManagerID: r.PlanManagerID,
		status:        r.Status, issueDate: r.IssueDate, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		convertedInvoiceID: r.ConvertedInvoiceID,
		businessSnap:       r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, participantName: r.ParticipantName,
	}
}

func estimateFieldsFromList(r gen.ListEstimatesRow) estimateFields {
	return estimateFields{
		id: r.ID, uuid: r.Uuid, number: r.Number, participantID: r.ParticipantID,
		planManagerID: r.PlanManagerID,
		status:        r.Status, issueDate: r.IssueDate, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		convertedInvoiceID: r.ConvertedInvoiceID,
		businessSnap:       r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, participantName: r.ParticipantName,
	}
}

func estimateFieldsFromStatus(r gen.ListEstimatesByStatusRow) estimateFields {
	return estimateFields{
		id: r.ID, uuid: r.Uuid, number: r.Number, participantID: r.ParticipantID,
		planManagerID: r.PlanManagerID,
		status:        r.Status, issueDate: r.IssueDate, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		convertedInvoiceID: r.ConvertedInvoiceID,
		businessSnap:       r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, participantName: r.ParticipantName,
	}
}

func estimateFieldsFromParticipant(r gen.ListParticipantEstimatesRow) estimateFields {
	return estimateFields{
		id: r.ID, uuid: r.Uuid, number: r.Number, participantID: r.ParticipantID,
		planManagerID: r.PlanManagerID,
		status:        r.Status, issueDate: r.IssueDate, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		convertedInvoiceID: r.ConvertedInvoiceID,
		businessSnap:       r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, participantName: r.ParticipantName,
	}
}

// mapEstimateLineItems maps generated rows to domain line items (non-nil).
func mapEstimateLineItems(rows []gen.EstimateLineItem) []*billing.LineItem {
	out := make([]*billing.LineItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toEstimateLineItem(rows[i]))
	}
	return out
}

// toEstimateLineItem maps one generated estimate line item to the shared
// LineItem domain shape.
func toEstimateLineItem(row gen.EstimateLineItem) *billing.LineItem {
	return &billing.LineItem{
		ID:               row.ID,
		UUID:             row.Uuid,
		SupportItemID:    db.PtrStr(row.SupportItemID),
		CustomItemID:     db.PtrID(row.CustomItemID),
		CatalogVersionID: db.PtrStr(row.CatalogVersionID),
		Code:             row.Code.String,
		Description:      row.Description,
		ServiceDate:      row.ServiceDate.String,
		Unit:             row.Unit.String,
		Quantity:         row.Quantity,
		UnitPrice:        row.UnitPrice,
		GstFree:          row.GstFree == 1,
		LineTotal:        row.LineTotal,
		SortOrder:        row.SortOrder.Int64,
	}
}

// orDefault returns s when non-empty, otherwise def.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
