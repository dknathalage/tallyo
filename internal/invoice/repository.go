package invoice

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
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/numbering"
	"github.com/google/uuid"
)

// invoiceListSelect mirrors the ListInvoices sqlc query body up to the WHERE.
// Keep in sync with internal/db/queries/invoices.sql. The tenant filter is the
// FIRST and ONLY ? in the base; listquery's c.Where is appended as " AND ...".
const invoiceListSelect = `SELECT i.*, p.name AS participant_name
FROM invoices i
LEFT JOIN participants p ON i.participant_id = p.id AND p.tenant_id = i.tenant_id
WHERE i.tenant_id = ?`

// InvoiceCols is the listquery allowlist for invoices. Keys match the JSON field
// names so the frontend column key drives filter, sort and display with one
// identifier. Invoices are a read-only document list (no drawer edit).
var InvoiceCols = listquery.Spec{
	"number":          {Col: "i.number", Filter: listquery.Text},
	"participantName": {Col: "p.name", Filter: listquery.Text},
	"status":          {Col: "i.status", Filter: listquery.Enum},
	"issueDate":       {Col: "i.issue_date", Filter: listquery.Date},
	"total":           {Col: "i.total", Filter: listquery.Number},
}

// Invoice is the domain view of an invoice with its resolved participant name
// and embedded line items.
type Invoice struct {
	ID               int64               `json:"id"`
	UUID             string              `json:"uuid"`
	Number           string              `json:"number"`
	ParticipantID    int64               `json:"participantId"`
	ParticipantName  string              `json:"participantName"`
	PlanManagerID    *int64              `json:"planManagerId"`
	Status           string              `json:"status"`
	IssueDate        string              `json:"issueDate"`
	DueDate          string              `json:"dueDate"`
	Subtotal         float64             `json:"subtotal"`
	Tax              float64             `json:"tax"`
	Total            float64             `json:"total"`
	Notes            string              `json:"notes"`
	BusinessSnapshot string              `json:"businessSnapshot"`
	ClientSnapshot   string              `json:"participantSnapshot"`
	PayerSnapshot    string              `json:"planManagerSnapshot"`
	CreatedAt        string              `json:"createdAt"`
	UpdatedAt        string              `json:"updatedAt"`
	TotalPaid        float64             `json:"totalPaid"`
	Balance          float64             `json:"balance"`
	LineItems        []*billing.LineItem `json:"lineItems"`
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
	ClientSnapshot   string  `json:"participantSnapshot"`
	PayerSnapshot    string  `json:"planManagerSnapshot"`
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
// the business profile, participant and plan manager.
func (r *InvoicesRepo) fillSnapshots(ctx context.Context, tenantID int64, in *InvoiceInput) {
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

// Create inserts an invoice plus its line items inside one numbering-retried
// transaction, audits the create, and re-reads the row. ParticipantID and at
// least one line item are required.
func (r *InvoicesRepo) Create(ctx context.Context, tenantID int64, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
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
func (r *InvoicesRepo) createTx(ctx context.Context, tenantID int64, in InvoiceInput, items []billing.LineItemInput, newID *int64) error {
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
	if err := InsertLineItems(ctx, q, tenantID, inv.ID, items); err != nil {
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

// draftShiftItem holds the validated facts about one shift that DraftFromShifts
// needs: its participant and the number of unbilled items it carries.
type draftShiftItem struct {
	shiftID       int64
	participantID int64
	itemCount     int64
}

// validateDraftShifts reads each shift (no writes) and enforces the draft
// preconditions: the shift exists for the tenant, is status 'recorded' with no
// invoice yet, carries at least one unbilled item (G5), and every shift shares
// one participant. Returns the shared participant id and the per-shift facts.
func (r *InvoicesRepo) validateDraftShifts(ctx context.Context, tenantID int64, shiftIDs []int64) (int64, []draftShiftItem, error) {
	if len(shiftIDs) == 0 {
		return 0, nil, errors.New("draft from shifts: at least one shift is required")
	}
	q := gen.New(r.db)
	var participantID int64
	facts := make([]draftShiftItem, 0, len(shiftIDs))
	for i := range shiftIDs { // bounded by len(shiftIDs)
		sh, err := q.GetShiftByID(ctx, gen.GetShiftByIDParams{TenantID: tenantID, ID: shiftIDs[i]})
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil, fmt.Errorf("draft from shifts: shift %d not found", shiftIDs[i])
		}
		if err != nil {
			return 0, nil, fmt.Errorf("draft from shifts: load shift %d: %w", shiftIDs[i], err)
		}
		if sh.Status != "recorded" || sh.InvoiceID.Valid {
			return 0, nil, fmt.Errorf("draft from shifts: shift %d is not recorded+unbilled", shiftIDs[i])
		}
		if i == 0 {
			participantID = sh.ParticipantID
		} else if sh.ParticipantID != participantID {
			return 0, nil, errors.New("draft from shifts: all shifts must share one participant")
		}
		n, err := q.CountShiftItems(ctx, gen.CountShiftItemsParams{TenantID: tenantID, ShiftID: sql.NullInt64{Int64: shiftIDs[i], Valid: true}})
		if err != nil {
			return 0, nil, fmt.Errorf("draft from shifts: count items %d: %w", shiftIDs[i], err)
		}
		if n == 0 {
			return 0, nil, fmt.Errorf("draft from shifts: shift %d has no items", shiftIDs[i])
		}
		facts = append(facts, draftShiftItem{shiftID: shiftIDs[i], participantID: sh.ParticipantID, itemCount: n})
	}
	return participantID, facts, nil
}

// DraftFromShifts creates a draft invoice header for participantID, links every
// validated shift's unbilled items onto it, and persists totals computed from
// the now-linked lines — all in ONE numbering-retried transaction. The shifts
// table is NOT written here; the caller advances the shifts to 'drafted'
// afterwards (a separate, post-commit step), mirroring Delete↔ClearForInvoice.
func (r *InvoicesRepo) DraftFromShifts(ctx context.Context, tenantID, participantID int64, facts []draftShiftItem) (*Invoice, error) {
	if tenantID == 0 || participantID == 0 {
		return nil, errors.New("draft from shifts: tenant and participant id required")
	}
	in := InvoiceInput{ParticipantID: participantID, Status: "draft"}
	now := time.Now().UTC().Format("2006-01-02")
	in.IssueDate = now
	in.DueDate = now
	r.fillSnapshots(ctx, tenantID, &in)

	var newID int64
	err := numbering.WithRetry(ctx, 10, func() error {
		return r.draftTx(ctx, tenantID, in, facts, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("draft from shifts: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// draftTx runs one draft attempt: allocate the number, insert a zero-total
// header, link each shift's items (assigning a sort_order base), recompute
// totals from the linked lines, persist them, and audit — all in one tx.
func (r *InvoicesRepo) draftTx(ctx context.Context, tenantID int64, in InvoiceInput, facts []draftShiftItem, newID *int64) error {
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
	inv, err := q.CreateInvoice(ctx, createInvoiceParams(tenantID, in, nil, num))
	if err != nil {
		return err
	}
	var sortBase int64
	for i := range facts { // bounded by len(facts)
		if e := q.LinkShiftItemsToInvoice(ctx, gen.LinkShiftItemsToInvoiceParams{
			InvoiceID: sql.NullInt64{Int64: inv.ID, Valid: true},
			SortOrder: sql.NullInt64{Int64: sortBase, Valid: true},
			TenantID:  tenantID,
			ShiftID:   sql.NullInt64{Int64: facts[i].shiftID, Valid: true},
		}); e != nil {
			return fmt.Errorf("link shift %d: %w", facts[i].shiftID, e)
		}
		sortBase += facts[i].itemCount
	}
	lines, err := q.ListLineItemsForInvoice(ctx, gen.ListLineItemsForInvoiceParams{
		TenantID: tenantID, InvoiceID: sql.NullInt64{Int64: inv.ID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("list linked lines: %w", err)
	}
	totals := totalsFromRows(lines)
	if _, e := q.UpdateInvoiceTotals(ctx, gen.UpdateInvoiceTotalsParams{
		Subtotal: totals.Subtotal, Tax: totals.Tax, Total: totals.Total,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339), TenantID: tenantID, ID: inv.ID,
	}); e != nil {
		return fmt.Errorf("update totals: %w", e)
	}
	if e := audit.Log(ctx, tx, audit.Entry{
		EntityType: "invoice", EntityID: inv.ID, Action: "create",
		Changes: audit.Changes(map[string]any{"number": num, "draftedFromShifts": len(facts)}),
	}); e != nil {
		return e
	}
	if e := tx.Commit(); e != nil {
		return e
	}
	*newID = inv.ID
	return nil
}

// totalsFromRows sums line totals from already-priced line_items rows. Tax is 0
// (NDIS GST-free lines carry no tax; gst-bearing lines already fold tax into
// their unit price upstream — same as the human invoice path).
func totalsFromRows(rows []gen.LineItem) billing.Totals {
	var subtotal float64
	for i := range rows { // bounded by len(rows)
		subtotal += billing.Round2(rows[i].LineTotal)
	}
	subtotal = billing.Round2(subtotal)
	return billing.Totals{Subtotal: subtotal, Tax: 0, Total: subtotal}
}

// NextInvoiceNumber allocates the next per-tenant invoice number ("INV-NNNN").
// Exported so that estimate and recurring repositories can reuse it.
func NextInvoiceNumber(ctx context.Context, q *gen.Queries, tenantID int64) (string, error) {
	const prefix = "INV-"
	max, err := q.MaxInvoiceNumberLike(ctx, gen.MaxInvoiceNumberLikeParams{
		PrefixLen: int64(len(prefix)),
		TenantID:  tenantID,
		Pattern:   prefix + "%",
	})
	if err != nil {
		return "", fmt.Errorf("next invoice number: %w", err)
	}
	return numbering.Format(prefix, max), nil
}

// createInvoiceParams builds the insert params, applying defaults (draft) and
// computing totals from the line items.
func createInvoiceParams(tenantID int64, in InvoiceInput, items []billing.LineItemInput, num string) gen.CreateInvoiceParams {
	t := billing.ComputeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		Uuid:             uuid.NewString(),
		TenantID:         tenantID,
		Number:           num,
		ParticipantID:    in.ParticipantID,
		PlanManagerID:    db.NullID(in.PlanManagerID),
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

// InsertLineItems writes each line item with its computed total. Bounded by len.
// Exported so that recurring repository can reuse it.
func InsertLineItems(ctx context.Context, q *gen.Queries, tenantID, invoiceID int64, items []billing.LineItemInput) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		_, err := q.CreateLineItem(ctx, gen.CreateLineItemParams{
			Uuid:             uuid.NewString(),
			TenantID:         tenantID,
			ShiftID:          sql.NullInt64{}, // invoice lines from this path are not shift items
			InvoiceID:        sql.NullInt64{Int64: invoiceID, Valid: true},
			SupportItemID:    db.NullStr(it.SupportItemID),
			CustomItemID:     db.NullID(it.CustomItemID),
			CatalogVersionID: db.NullStr(it.CatalogVersionID),
			Code:             db.NzMaybe(it.Code),
			Description:      it.Description,
			ServiceDate:      db.NzMaybe(it.ServiceDate),
			Unit:             db.NzMaybe(it.Unit),
			StartTime:        db.NzMaybe(it.StartTime),
			EndTime:          db.NzMaybe(it.EndTime),
			Quantity:         it.Quantity,
			UnitPrice:        it.UnitPrice,
			GstFree:          db.B2i(it.GstFree),
			LineTotal:        billing.Round2(it.Quantity * it.UnitPrice),
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
	rows, err := q.ListLineItemsForInvoice(ctx, gen.ListLineItemsForInvoiceParams{TenantID: tenantID, InvoiceID: sql.NullInt64{Int64: id, Valid: true}})
	if err != nil {
		return nil, fmt.Errorf("list line items: %w", err)
	}
	inv.LineItems = mapLineItems(rows)
	tp, err := q.InvoiceTotalPaid(ctx, gen.InvoiceTotalPaidParams{TenantID: tenantID, InvoiceID: id})
	if err != nil {
		return nil, fmt.Errorf("invoice total paid: %w", err)
	}
	inv.TotalPaid = tp
	inv.Balance = billing.Round2(inv.Total - tp)
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

// Query returns one page of invoices (header only) plus the total row count for
// the filter (ignoring pagination). The clause is built by listquery from an
// allowlisted spec, so its Where/Order fragments are injection-safe. Default
// order (no sort requested) is newest first, matching ListInvoices.
func (r *InvoicesRepo) Query(ctx context.Context, tenantID int64, c listquery.Clause) ([]*Invoice, int64, error) {
	if tenantID == 0 {
		return nil, 0, errors.New("query invoices: tenant id required")
	}
	var total int64
	countSQL := "SELECT count(*) FROM (" + invoiceListSelect + c.Where + ")"
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count invoices: %w", err)
	}
	order := c.Order
	if order == "" {
		order = " ORDER BY i.created_at DESC"
	}
	sqlText := invoiceListSelect + c.Where + order + c.Limit
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query invoices: %w", err)
	}
	defer rows.Close()
	out := make([]*Invoice, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var f invoiceFields
		var tenant int64
		if err := rows.Scan(&f.id, &f.uuid, &tenant, &f.number, &f.participantID,
			&f.planManagerID, &f.status, &f.issueDate, &f.dueDate, &f.subtotal,
			&f.tax, &f.total, &f.notes, &f.businessSnap, &f.clientSnap, &f.payerSnap,
			&f.createdAt, &f.updatedAt, &f.participantName); err != nil {
			return nil, 0, fmt.Errorf("scan invoice: %w", err)
		}
		out = append(out, toInvoiceFromRow(f))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query invoices: %w", err)
	}
	return out, total, nil
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
func (r *InvoicesRepo) Update(ctx context.Context, tenantID, id int64, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
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
		if e := q.DeleteLineItemsForInvoice(ctx, gen.DeleteLineItemsForInvoiceParams{TenantID: tenantID, InvoiceID: sql.NullInt64{Int64: id, Valid: true}}); e != nil {
			return fmt.Errorf("clear items: %w", e)
		}
		return InsertLineItems(ctx, q, tenantID, id, items)
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
func updateInvoiceParams(tenantID int64, in InvoiceInput, items []billing.LineItemInput, number string, id int64) gen.UpdateInvoiceParams {
	t := billing.ComputeTotals(items, in.Tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpdateInvoiceParams{
		Number:           number,
		ParticipantID:    in.ParticipantID,
		PlanManagerID:    db.NullID(in.PlanManagerID),
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

// Delete removes an invoice and writes one audit row. Shift items are unlinked
// (invoice_id→NULL) BEFORE the delete so the line_items.invoice_id ON DELETE
// CASCADE removes only shift-less manual lines; shift items survive (shift_id
// intact) and return to their shift. Unlink + cascade are atomic in one tx.
func (r *InvoicesRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		if e := q.UnlinkShiftItemsFromInvoice(ctx, gen.UnlinkShiftItemsFromInvoiceParams{
			TenantID: tenantID, InvoiceID: sql.NullInt64{Int64: id, Valid: true},
		}); e != nil {
			return fmt.Errorf("unlink shift items: %w", e)
		}
		if e := q.DeleteInvoice(ctx, gen.DeleteInvoiceParams{TenantID: tenantID, ID: id}); e != nil {
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
			if e := q.UnlinkShiftItemsFromInvoice(ctx, gen.UnlinkShiftItemsFromInvoiceParams{
				TenantID: tenantID, InvoiceID: sql.NullInt64{Int64: id, Valid: true},
			}); e != nil {
				return fmt.Errorf("unlink shift items %d: %w", id, e)
			}
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

// MarkOverdueForTenant flips every 'sent' invoice of one tenant whose due date
// has passed to 'overdue', auditing each, atomically. Returns the affected
// invoices. This is the per-tenant sweep path (spec §8): the caller iterates
// active tenants and skips suspended ones.
func (r *InvoicesRepo) MarkOverdueForTenant(ctx context.Context, tenantID int64) ([]OverdueInvoice, error) {
	if tenantID == 0 {
		return nil, errors.New("mark overdue: tenant id required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mark overdue: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	rows, err := q.SelectOverdueInvoicesForTenant(ctx, tenantID)
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

// ActiveTenantIDs returns the ids of tenants whose status is 'active' (suspended
// tenants are excluded), used by the per-tenant sweeps.
func (r *InvoicesRepo) ActiveTenantIDs(ctx context.Context) ([]int64, error) {
	ids, err := gen.New(r.db).ListActiveTenantIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("active tenant ids: %w", err)
	}
	return ids, nil
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

// Exists reports whether the tenant has an invoice with the given id.
// It satisfies the shift.InvoiceChecker interface so the shift service can
// verify invoice ownership without importing the invoice package.
func (r *InvoicesRepo) Exists(ctx context.Context, tenantID, invoiceID int64) (bool, error) {
	inv, err := r.Get(ctx, tenantID, invoiceID)
	if err != nil {
		return false, err
	}
	return inv != nil, nil
}

// ResolveParticipantID translates a participant uuid into its int PK, scoped to
// the tenant. Returns (0, nil) when no participant matches (caller 404s).
func (r *InvoicesRepo) ResolveParticipantID(ctx context.Context, tenantID int64, participantUUID string) (int64, error) {
	id, err := gen.New(r.db).GetParticipantIDByUUID(ctx, gen.GetParticipantIDByUUIDParams{TenantID: tenantID, Uuid: participantUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("resolve participant uuid: %w", err)
	}
	return id, nil
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
		PlanManagerID:    db.PtrID(f.planManagerID),
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
		LineItems:        []*billing.LineItem{},
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
func mapLineItems(rows []gen.LineItem) []*billing.LineItem {
	out := make([]*billing.LineItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, billing.LineItemFromRow(rows[i]))
	}
	return out
}

// orDefault returns s when non-empty, otherwise def.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
