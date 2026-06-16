package repository

// NOTE (J4): rewritten to the NDIS invoice/line-item domain (spec §4.2). The
// header no longer carries payment_terms / currency / tax_rate / tax_rate_id;
// it carries participant_id, optional plan_manager_id, and subtotal/tax/total.
// Line items carry NDIS fields: code, service_date, unit, unit_price, gst_free,
// line_total, and optional support_item_id / custom_item_id / catalog_version_id.
//
// Design decisions (deferred concerns belong to J8/J10):
//   - `tax` is supplied on the header input (computed upstream by the J10
//     validation engine). This repo only sums line totals → subtotal and
//     subtotal+tax → total, rounding to the cent at each boundary (spec §6 money
//     note). It does NOT perform price-cap / plan-window validation (J10).
//   - Per-tenant document numbering is allocated inline via the tenant-scoped
//     gen.MaxInvoiceNumberLike query inside the create tx, wrapped in
//     numbering.WithRetry (the numbering package's schema-specific Config path is
//     superseded; full per-tenant numbering hardening is J11).

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

// Invoice is the domain view of an invoice with its resolved participant name
// and embedded line items.
type Invoice struct {
	ID               int64       `json:"id"`
	UUID             string      `json:"uuid"`
	Number           string      `json:"number"`
	ParticipantID    int64       `json:"participantId"`
	ParticipantName  string      `json:"participantName"`
	PlanManagerID    *int64      `json:"planManagerId"`
	Status           string      `json:"status"`
	IssueDate        string      `json:"issueDate"`
	DueDate          string      `json:"dueDate"`
	Subtotal         float64     `json:"subtotal"`
	Tax              float64     `json:"tax"`
	Total            float64     `json:"total"`
	Notes            string      `json:"notes"`
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
	ID               int64   `json:"id"`
	UUID             string  `json:"uuid"`
	SupportItemID    *int64  `json:"supportItemId"`
	CustomItemID     *int64  `json:"customItemId"`
	CatalogVersionID *int64  `json:"catalogVersionId"`
	Code             string  `json:"code"`
	Description      string  `json:"description"`
	ServiceDate      string  `json:"serviceDate"`
	Unit             string  `json:"unit"`
	Quantity         float64 `json:"quantity"`
	UnitPrice        float64 `json:"unitPrice"`
	GstFree          bool    `json:"gstFree"`
	LineTotal        float64 `json:"lineTotal"`
	SortOrder        int64   `json:"sortOrder"`
}

// InvoiceInput is the writable subset of an invoice header. Snapshot fields,
// when non-empty, are stored verbatim; when empty, defaults are built from the
// business profile, participant and plan manager.
type InvoiceInput struct {
	ParticipantID    int64   `json:"participantId"`
	PlanManagerID    *int64  `json:"planManagerId"`
	Status           string  `json:"status"`
	IssueDate        string  `json:"issueDate"`
	DueDate          string  `json:"dueDate"`
	Tax              float64 `json:"tax"`
	Notes            string  `json:"notes"`
	BusinessSnapshot string  `json:"businessSnapshot"`
	ClientSnapshot   string  `json:"clientSnapshot"`
	PayerSnapshot    string  `json:"payerSnapshot"`
}

// LineItemInput is the writable subset of a line item. LineTotal is computed
// (round2(quantity*unitPrice)) when not explicitly supplied.
type LineItemInput struct {
	SupportItemID    *int64  `json:"supportItemId"`
	CustomItemID     *int64  `json:"customItemId"`
	CatalogVersionID *int64  `json:"catalogVersionId"`
	Code             string  `json:"code"`
	Description      string  `json:"description"`
	ServiceDate      string  `json:"serviceDate"`
	Unit             string  `json:"unit"`
	Quantity         float64 `json:"quantity"`
	UnitPrice        float64 `json:"unitPrice"`
	GstFree          bool    `json:"gstFree"`
	SortOrder        int64   `json:"sortOrder"`
}

// OverdueInvoice identifies an invoice flipped to overdue by MarkOverdue.
type OverdueInvoice struct {
	ID       int64  `json:"id"`
	TenantID int64  `json:"tenantId"`
	Number   string `json:"number"`
}

// ParticipantStats aggregates a participant's invoice activity.
type ParticipantStats struct {
	InvoiceCount  int64   `json:"invoiceCount"`
	TotalInvoiced float64 `json:"totalInvoiced"`
	TotalPaid     float64 `json:"totalPaid"`
}

// InvoicesRepo reads and writes the invoices + line_items tables (tenant-scoped).
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
	subtotal float64
	tax      float64
	total    float64
}

// computeTotals sums line totals into the subtotal and applies the (already
// computed) tax amount. Each boundary is rounded to the cent (spec §6).
func computeTotals(items []LineItemInput, tax float64) totals {
	var subtotal float64
	for i := range items { // bounded by len(items)
		subtotal += round2(items[i].Quantity * items[i].UnitPrice)
	}
	subtotal = round2(subtotal)
	tax = round2(tax)
	return totals{subtotal: subtotal, tax: tax, total: round2(subtotal + tax)}
}

// round2 rounds to two decimal places (cents).
func round2(x float64) float64 {
	return math.Round(x*100) / 100
}

// fillSnapshots fills any empty snapshot field on in with a default built from
// the business profile, participant and plan manager.
func (r *InvoicesRepo) fillSnapshots(ctx context.Context, tenantID int64, in *InvoiceInput) {
	if in.BusinessSnapshot == "" {
		in.BusinessSnapshot = r.buildBusinessSnapshot(ctx, tenantID)
	}
	if in.ClientSnapshot == "" {
		in.ClientSnapshot = r.buildParticipantSnapshot(ctx, tenantID, in.ParticipantID)
	}
	if in.PayerSnapshot == "" {
		in.PayerSnapshot = r.buildPlanManagerSnapshot(ctx, tenantID, in.PlanManagerID)
	}
}

// Create inserts an invoice plus its line items inside one numbering-retried
// transaction, audits the create, and re-reads the row. ParticipantID and at
// least one line item are required.
func (r *InvoicesRepo) Create(ctx context.Context, tenantID int64, in InvoiceInput, items []LineItemInput) (*Invoice, error) {
	if tenantID == 0 {
		return nil, errors.New("create invoice: tenant id required")
	}
	if in.ParticipantID == 0 {
		return nil, errors.New("create invoice: participant is required")
	}
	if len(items) == 0 {
		return nil, errors.New("create invoice: at least one line item is required")
	}
	r.fillSnapshots(ctx, tenantID, &in)

	var newID int64
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
func (r *InvoicesRepo) createTx(ctx context.Context, tenantID int64, in InvoiceInput, items []LineItemInput, newID *int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	num, err := nextInvoiceNumber(ctx, q, tenantID)
	if err != nil {
		return err
	}
	inv, err := q.CreateInvoice(ctx, createInvoiceParams(tenantID, in, items, num))
	if err != nil {
		return err
	}
	if err := insertItems(ctx, q, tenantID, inv.ID, items); err != nil {
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

// nextInvoiceNumber allocates the next per-tenant invoice number ("INV-NNNN").
func nextInvoiceNumber(ctx context.Context, q *gen.Queries, tenantID int64) (string, error) {
	const prefix = "INV-"
	max, err := q.MaxInvoiceNumberLike(ctx, gen.MaxInvoiceNumberLikeParams{
		PrefixLen: int64(len(prefix)),
		TenantID:  tenantID,
		Pattern:   prefix + "%",
	})
	if err != nil {
		return "", fmt.Errorf("next invoice number: %w", err)
	}
	return fmt.Sprintf("%s%04d", prefix, max+1), nil
}

// createInvoiceParams builds the insert params, applying defaults (draft) and
// computing totals from the line items.
func createInvoiceParams(tenantID int64, in InvoiceInput, items []LineItemInput, num string) gen.CreateInvoiceParams {
	t := computeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		Uuid:             uuid.NewString(),
		TenantID:         tenantID,
		Number:           num,
		ParticipantID:    in.ParticipantID,
		PlanManagerID:    nullID(in.PlanManagerID),
		Status:           orDefault(in.Status, "draft"),
		IssueDate:        in.IssueDate,
		DueDate:          in.DueDate,
		Subtotal:         t.subtotal,
		Tax:              t.tax,
		Total:            t.total,
		Notes:            nzMaybe(in.Notes),
		BusinessSnapshot: nzMaybe(in.BusinessSnapshot),
		ClientSnapshot:   nzMaybe(in.ClientSnapshot),
		PayerSnapshot:    nzMaybe(in.PayerSnapshot),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// insertItems writes each line item with its computed total. Bounded by len.
func insertItems(ctx context.Context, q *gen.Queries, tenantID, invoiceID int64, items []LineItemInput) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		_, err := q.CreateLineItem(ctx, gen.CreateLineItemParams{
			Uuid:             uuid.NewString(),
			TenantID:         tenantID,
			InvoiceID:        invoiceID,
			SupportItemID:    nullID(it.SupportItemID),
			CustomItemID:     nullID(it.CustomItemID),
			CatalogVersionID: nullID(it.CatalogVersionID),
			Code:             nzMaybe(it.Code),
			Description:      it.Description,
			ServiceDate:      nzMaybe(it.ServiceDate),
			Unit:             nzMaybe(it.Unit),
			Quantity:         it.Quantity,
			UnitPrice:        it.UnitPrice,
			GstFree:          b2i(it.GstFree),
			LineTotal:        round2(it.Quantity * it.UnitPrice),
			SortOrder:        sql.NullInt64{Int64: it.SortOrder, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("insert line item %d: %w", i, err)
		}
	}
	return nil
}

// Get returns the invoice (with participant name and line items), or (nil, nil)
// when absent.
func (r *InvoicesRepo) Get(ctx context.Context, tenantID, id int64) (*Invoice, error) {
	q := gen.New(r.db)
	row, err := q.GetInvoice(ctx, gen.GetInvoiceParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	inv := toInvoiceFromRow(invoiceFieldsFromGet(row))
	rows, err := q.ListLineItems(ctx, gen.ListLineItemsParams{TenantID: tenantID, InvoiceID: id})
	if err != nil {
		return nil, fmt.Errorf("list line items: %w", err)
	}
	inv.LineItems = mapLineItems(rows)
	tp, err := q.InvoiceTotalPaid(ctx, gen.InvoiceTotalPaidParams{TenantID: tenantID, InvoiceID: id})
	if err != nil {
		return nil, fmt.Errorf("invoice total paid: %w", err)
	}
	inv.TotalPaid = tp
	inv.Balance = round2(inv.Total - tp)
	return inv, nil
}

// List returns every invoice for the tenant (header only), newest first.
func (r *InvoicesRepo) List(ctx context.Context, tenantID int64) ([]*Invoice, error) {
	rows, err := gen.New(r.db).ListInvoices(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	out := make([]*Invoice, 0, len(rows))
	for i := range rows {
		out = append(out, toInvoiceFromRow(invoiceFieldsFromList(rows[i])))
	}
	return out, nil
}

// ListByStatus returns the tenant's invoices with the given status.
func (r *InvoicesRepo) ListByStatus(ctx context.Context, tenantID int64, status string) ([]*Invoice, error) {
	rows, err := gen.New(r.db).ListInvoicesByStatus(ctx, gen.ListInvoicesByStatusParams{
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return nil, fmt.Errorf("list invoices by status: %w", err)
	}
	out := make([]*Invoice, 0, len(rows))
	for i := range rows {
		out = append(out, toInvoiceFromRow(invoiceFieldsFromStatus(rows[i])))
	}
	return out, nil
}

// ListParticipantInvoices returns one participant's invoices (header only).
func (r *InvoicesRepo) ListParticipantInvoices(ctx context.Context, tenantID, participantID int64) ([]*Invoice, error) {
	rows, err := gen.New(r.db).ListParticipantInvoices(ctx, gen.ListParticipantInvoicesParams{
		TenantID:      tenantID,
		ParticipantID: participantID,
	})
	if err != nil {
		return nil, fmt.Errorf("list participant invoices: %w", err)
	}
	out := make([]*Invoice, 0, len(rows))
	for i := range rows {
		out = append(out, toInvoiceFromRow(invoiceFieldsFromParticipant(rows[i])))
	}
	return out, nil
}

// Update rewrites the header (recomputing totals) and replaces all line items,
// atomically with one audit row. Empty snapshot inputs keep the existing stored
// snapshots. Returns (nil, nil) when the invoice does not exist.
func (r *InvoicesRepo) Update(ctx context.Context, tenantID, id int64, in InvoiceInput, items []LineItemInput) (*Invoice, error) {
	if in.ParticipantID == 0 {
		return nil, errors.New("update invoice: participant is required")
	}
	if len(items) == 0 {
		return nil, errors.New("update invoice: at least one line item is required")
	}
	existing, err := gen.New(r.db).GetInvoice(ctx, gen.GetInvoiceParams{TenantID: tenantID, ID: id})
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
		if e := q.DeleteLineItemsForInvoice(ctx, gen.DeleteLineItemsForInvoiceParams{TenantID: tenantID, InvoiceID: id}); e != nil {
			return fmt.Errorf("clear items: %w", e)
		}
		return insertItems(ctx, q, tenantID, id, items)
	})
	if err != nil {
		return nil, fmt.Errorf("update invoice: %w", err)
	}
	return r.Get(ctx, tenantID, id)
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

// updateInvoiceParams builds the update params, recomputing totals from items.
// The document number is immutable, so the existing number is preserved.
func updateInvoiceParams(tenantID int64, in InvoiceInput, items []LineItemInput, number string, id int64) gen.UpdateInvoiceParams {
	t := computeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpdateInvoiceParams{
		Number:           number,
		ParticipantID:    in.ParticipantID,
		PlanManagerID:    nullID(in.PlanManagerID),
		Status:           orDefault(in.Status, "draft"),
		IssueDate:        in.IssueDate,
		DueDate:          in.DueDate,
		Subtotal:         t.subtotal,
		Tax:              t.tax,
		Total:            t.total,
		Notes:            nzMaybe(in.Notes),
		BusinessSnapshot: nzMaybe(in.BusinessSnapshot),
		ClientSnapshot:   nzMaybe(in.ClientSnapshot),
		PayerSnapshot:    nzMaybe(in.PayerSnapshot),
		UpdatedAt:        now,
		TenantID:         tenantID,
		ID:               id,
	}
}

// UpdateStatus sets just the status column, atomically with one audit row.
func (r *InvoicesRepo) UpdateStatus(ctx context.Context, tenantID, id int64, status string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if e := gen.New(tx).UpdateInvoiceStatus(ctx, gen.UpdateInvoiceStatusParams{
			Status: status, UpdatedAt: now, TenantID: tenantID, ID: id,
		}); e != nil {
			return fmt.Errorf("update status: %w", e)
		}
		return nil
	})
}

// Delete removes an invoice (line items cascade) and writes one audit row.
func (r *InvoicesRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).DeleteInvoice(ctx, gen.DeleteInvoiceParams{TenantID: tenantID, ID: id}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return nil
	})
}

// BulkDelete removes several invoices and writes one audit row. Empty is a no-op.
func (r *InvoicesRepo) BulkDelete(ctx context.Context, tenantID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if e := q.DeleteInvoice(ctx, gen.DeleteInvoiceParams{TenantID: tenantID, ID: id}); e != nil {
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
func (r *InvoicesRepo) BulkUpdateStatus(ctx context.Context, tenantID int64, ids []int64, status string) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		now := time.Now().UTC().Format(time.RFC3339)
		for _, id := range ids { // bounded by len(ids)
			if e := q.UpdateInvoiceStatus(ctx, gen.UpdateInvoiceStatusParams{
				Status: status, UpdatedAt: now, TenantID: tenantID, ID: id,
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

// MarkOverdue flips every 'sent' invoice whose due date has passed to 'overdue',
// across ALL tenants, auditing each, atomically. Returns the affected invoices.
// This is the launch/hourly sweep path; per-tenant iteration is J11's concern.
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
		if e := flipOverdue(ctx, tx, q, rows[i].TenantID, rows[i].ID, now); e != nil {
			return nil, fmt.Errorf("mark overdue: %w", e)
		}
		out = append(out, OverdueInvoice{ID: rows[i].ID, TenantID: rows[i].TenantID, Number: rows[i].Number})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("mark overdue: commit: %w", err)
	}
	return out, nil
}

// flipOverdue sets one invoice to overdue and logs the transition.
func flipOverdue(ctx context.Context, tx *sql.Tx, q *gen.Queries, tenantID, id int64, now string) error {
	if e := q.UpdateInvoiceStatus(ctx, gen.UpdateInvoiceStatusParams{
		Status: "overdue", UpdatedAt: now, TenantID: tenantID, ID: id,
	}); e != nil {
		return e
	}
	return audit.Log(ctx, tx, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"from": "sent", "to": "overdue"}),
	})
}

// ParticipantStats returns the count and summed totals of a participant's
// invoices.
func (r *InvoicesRepo) ParticipantStats(ctx context.Context, tenantID, participantID int64) (*ParticipantStats, error) {
	row, err := gen.New(r.db).ParticipantInvoiceStats(ctx, gen.ParticipantInvoiceStatsParams{
		TenantID:      tenantID,
		ParticipantID: participantID,
	})
	if err != nil {
		return nil, fmt.Errorf("participant stats: %w", err)
	}
	return &ParticipantStats{InvoiceCount: row.InvoiceCount, TotalInvoiced: row.TotalInvoiced, TotalPaid: row.TotalPaid}, nil
}

// snapshotJSON builds the default snapshot JSON for an entity. metadata is parsed
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

// buildBusinessSnapshot reads the tenant's business profile and renders a default
// snapshot.
func (r *InvoicesRepo) buildBusinessSnapshot(ctx context.Context, tenantID int64) string {
	bp, err := gen.New(r.db).GetBusinessProfile(ctx, tenantID)
	if err != nil {
		return "{}"
	}
	return snapshotJSON(bp.Name, bp.Email.String, bp.Phone.String, bp.Address.String, bp.Metadata.String)
}

// buildParticipantSnapshot reads the participant and renders a default snapshot.
func (r *InvoicesRepo) buildParticipantSnapshot(ctx context.Context, tenantID, participantID int64) string {
	p, err := gen.New(r.db).GetParticipant(ctx, gen.GetParticipantParams{TenantID: tenantID, ID: participantID})
	if err != nil {
		return "{}"
	}
	return snapshotJSON(p.Name, p.Email.String, p.Phone.String, p.Address.String, p.Metadata.String)
}

// buildPlanManagerSnapshot renders a default snapshot for the given plan manager,
// or "{}" when none is set.
func (r *InvoicesRepo) buildPlanManagerSnapshot(ctx context.Context, tenantID int64, planManagerID *int64) string {
	if planManagerID == nil {
		return "{}"
	}
	pm, err := gen.New(r.db).GetPlanManager(ctx, gen.GetPlanManagerParams{TenantID: tenantID, ID: *planManagerID})
	if err != nil {
		return "{}"
	}
	return snapshotJSON(pm.Name, pm.Email.String, pm.Phone.String, pm.Address.String, pm.Metadata.String)
}

// invoiceFields is the shared, flat shape of every invoices join row (List,
// ListByStatus, ListParticipantInvoices and Get all produce identical structs
// under distinct gen type names, each adding ParticipantName).
type invoiceFields struct {
	id                                  int64
	uuid, number                        string
	participantID                       int64
	planManagerID                       sql.NullInt64
	status, issueDate, dueDate          string
	subtotal, tax, total                float64
	notes                               sql.NullString
	businessSnap, clientSnap, payerSnap sql.NullString
	createdAt, updatedAt                string
	participantName                     sql.NullString
}

// toInvoiceFromRow builds a domain Invoice (without line items) from the
// unwrapped join columns. LineItems defaults to a non-nil empty slice.
func toInvoiceFromRow(f invoiceFields) *Invoice {
	return &Invoice{
		ID:               f.id,
		UUID:             f.uuid,
		Number:           f.number,
		ParticipantID:    f.participantID,
		ParticipantName:  f.participantName.String,
		PlanManagerID:    ptrID(f.planManagerID),
		Status:           f.status,
		IssueDate:        f.issueDate,
		DueDate:          f.dueDate,
		Subtotal:         f.subtotal,
		Tax:              f.tax,
		Total:            f.total,
		Notes:            f.notes.String,
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
		id: r.ID, uuid: r.Uuid, number: r.Number, participantID: r.ParticipantID,
		planManagerID: r.PlanManagerID,
		status:        r.Status, issueDate: r.IssueDate, dueDate: r.DueDate,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, participantName: r.ParticipantName,
	}
}

func invoiceFieldsFromList(r gen.ListInvoicesRow) invoiceFields {
	return invoiceFields{
		id: r.ID, uuid: r.Uuid, number: r.Number, participantID: r.ParticipantID,
		planManagerID: r.PlanManagerID,
		status:        r.Status, issueDate: r.IssueDate, dueDate: r.DueDate,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, participantName: r.ParticipantName,
	}
}

func invoiceFieldsFromStatus(r gen.ListInvoicesByStatusRow) invoiceFields {
	return invoiceFields{
		id: r.ID, uuid: r.Uuid, number: r.Number, participantID: r.ParticipantID,
		planManagerID: r.PlanManagerID,
		status:        r.Status, issueDate: r.IssueDate, dueDate: r.DueDate,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, participantName: r.ParticipantName,
	}
}

func invoiceFieldsFromParticipant(r gen.ListParticipantInvoicesRow) invoiceFields {
	return invoiceFields{
		id: r.ID, uuid: r.Uuid, number: r.Number, participantID: r.ParticipantID,
		planManagerID: r.PlanManagerID,
		status:        r.Status, issueDate: r.IssueDate, dueDate: r.DueDate,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, participantName: r.ParticipantName,
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
		ID:               row.ID,
		UUID:             row.Uuid,
		SupportItemID:    ptrID(row.SupportItemID),
		CustomItemID:     ptrID(row.CustomItemID),
		CatalogVersionID: ptrID(row.CatalogVersionID),
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
