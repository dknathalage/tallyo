package recurring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/numbering"
)

// readGeneratedInvoice re-reads a just-created invoice (by row id) through the
// central db/gen + shared billing mappers and assembles the GeneratedInvoiceDoc.
// Returns (nil, nil) when the row is absent. This is a cross-domain READ via gen
// (allowed by the conventions), not a call into the invoice slice.
func (r *Repo) readGeneratedInvoice(ctx context.Context, tenantID, invID string) (*GeneratedInvoiceDoc, error) {
	q := gen.New(r.db)
	row, err := q.GetInvoiceByID(ctx, gen.GetInvoiceByIDParams{TenantID: tenantID, ID: invID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read generated invoice: %w", err)
	}
	lines, err := q.ListLineItemsForInvoice(ctx, gen.ListLineItemsForInvoiceParams{
		TenantID: tenantID, InvoiceID: sql.NullString{String: invID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("read generated invoice lines: %w", err)
	}
	tp, err := q.InvoiceTotalPaid(ctx, gen.InvoiceTotalPaidParams{TenantID: tenantID, InvoiceID: invID})
	if err != nil {
		return nil, fmt.Errorf("read generated invoice paid: %w", err)
	}
	return mapGeneratedInvoice(row, lines, tp), nil
}

// mapGeneratedInvoice assembles the domain doc from the central gen row, its
// line items, and the total paid. Line items map via the shared billing helpers.
func mapGeneratedInvoice(row gen.GetInvoiceByIDRow, lines []gen.ListLineItemsForInvoiceRow, totalPaid float64) *GeneratedInvoiceDoc {
	items := make([]*billing.LineItem, 0, len(lines))
	for i := range lines { // bounded by len(lines)
		items = append(items, billing.LineItemFromRow(billing.LineItemRowFromInvoice(lines[i])))
	}
	return &GeneratedInvoiceDoc{
		ID:               row.ID,
		Number:           row.Number,
		ClientUUID:       row.ClientUuid.String,
		ClientName:       row.ClientName.String,
		PayerUUID:        genNullStrPtr(row.PayerUuid),
		Status:           row.Status,
		IssueDate:        row.IssueDate,
		DueDate:          row.DueDate,
		Subtotal:         row.Subtotal,
		Tax:              row.Tax,
		Total:            row.Total,
		Notes:            row.Notes.String,
		BusinessSnapshot: row.BusinessSnapshot.String,
		ClientSnapshot:   row.ClientSnapshot.String,
		PayerSnapshot:    row.PayerSnapshot.String,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		TotalPaid:        totalPaid,
		Balance:          billing.Round2(row.Total - totalPaid),
		LineItems:        items,
	}
}

// AdvanceDate returns date advanced by one period of freq, in YYYY-MM-DD.
func (r *Repo) AdvanceDate(date, freq string) (string, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", fmt.Errorf("advance date: %w", err)
	}
	switch freq {
	case "weekly":
		t = t.AddDate(0, 0, 7)
	case "monthly":
		t = t.AddDate(0, 1, 0)
	case "quarterly":
		t = t.AddDate(0, 3, 0)
	default:
		return "", fmt.Errorf("unknown frequency %q", freq)
	}
	return t.Format("2006-01-02"), nil
}

// GenerateOne creates a draft invoice from the template (by uuid) AND advances
// next_due in the same transaction (idempotent re-runs). Returns (nil, nil) when
// the template is missing. The returned invoice is re-read after commit through
// the central db/gen (no invoice-slice import).
func (r *Repo) GenerateOne(ctx context.Context, tenantID string, templateUUID string) (*GeneratedInvoiceDoc, error) {
	tpl, err := r.Get(ctx, tenantID, templateUUID)
	if err != nil {
		return nil, fmt.Errorf("generate one: %w", err)
	}
	if tpl == nil {
		return nil, nil
	}
	items := parseLines(tpl.LineItems)
	today := time.Now().UTC().Format("2006-01-02")
	// tax_rate is a percentage; compute the tax amount from the subtotal.
	subtotal := billing.ComputeTotals(items, 0).Subtotal
	tax := billing.Round2(subtotal * (tpl.TaxRate / 100))
	snaps := r.buildGenSnapshots(ctx, tenantID, tpl.clientID, tpl.PayerID)

	var invID string
	err = numbering.WithRetry(ctx, 10, func() error {
		return r.generateTx(ctx, tenantID, tpl, items, today, tax, snaps, &invID)
	})
	if err != nil {
		return nil, fmt.Errorf("generate one: %w", err)
	}
	return r.readGeneratedInvoice(ctx, tenantID, invID)
}

// genSnapshots holds the default snapshot JSON for a generated invoice.
type genSnapshots struct {
	business string
	client   string
	payer    string
}

// buildGenSnapshots builds default snapshots for a generated invoice.
func (r *Repo) buildGenSnapshots(ctx context.Context, tenantID string, clientID, payerID *string) genSnapshots {
	var pid string
	if clientID != nil {
		pid = *clientID
	}
	return genSnapshots{
		business: r.snap.Business(ctx, tenantID),
		client:   r.snap.Client(ctx, tenantID, pid),
		payer:    r.snap.Payer(ctx, tenantID, payerID),
	}
}

// generateTx runs one generation attempt in one transaction for idempotency.
func (r *Repo) generateTx(ctx context.Context, tenantID string, tpl *RecurringTemplate, items []billing.LineItemInput,
	today string, tax float64, snaps genSnapshots, invID *string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	num, err := billing.NextNumber(ctx, q, tenantID, "INV-")
	if err != nil {
		return err
	}
	inv, err := q.CreateInvoice(ctx, recurringInvoiceParams(tenantID, tpl, items, today, tax, snaps, num))
	if err != nil {
		return err
	}
	if err := billing.InsertLineItems(ctx, q, tenantID, inv.ID, items); err != nil {
		return err
	}
	newDue, err := r.AdvanceDate(tpl.NextDue, tpl.Frequency)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := q.SetRecurringNextDue(ctx, gen.SetRecurringNextDueParams{
		NextDue: newDue, UpdatedAt: now, TenantID: tenantID, ID: tpl.ID,
	}); err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Entry{
		EntityType: "invoice", EntityID: inv.ID, Action: "create",
		Context: "from recurring template: " + tpl.Name,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*invID = inv.ID
	return nil
}

// recurringInvoiceParams maps a template + computed totals onto invoice create
// params: a draft invoice dated today with default snapshots.
func recurringInvoiceParams(tenantID string, tpl *RecurringTemplate, items []billing.LineItemInput, today string, tax float64, snaps genSnapshots, num string) gen.CreateInvoiceParams {
	var pid string
	if tpl.clientID != nil {
		pid = *tpl.clientID
	}
	t := billing.ComputeTotals(items, tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		ID:               ids.New(),
		TenantID:         tenantID,
		Number:           num,
		ClientID:         pid,
		PayerID:          db.NullStr(tpl.PayerID),
		Status:           "draft",
		IssueDate:        today,
		DueDate:          today,
		Subtotal:         t.Subtotal,
		Tax:              t.Tax,
		Total:            t.Total,
		Notes:            db.NzMaybe(tpl.Notes),
		BusinessSnapshot: db.NzMaybe(snaps.business),
		ClientSnapshot:   db.NzMaybe(snaps.client),
		PayerSnapshot:    db.NzMaybe(snaps.payer),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// GenerateDueForTenant generates one invoice per due template of ONE tenant
// whose next_due has passed. Idempotent: each generation advances next_due in
// its own transaction. Returns a non-nil slice. This is the per-tenant sweep
// path (spec §8): the caller iterates active tenants and skips suspended ones.
func (r *Repo) GenerateDueForTenant(ctx context.Context, tenantID string) ([]GeneratedInvoice, error) {
	if tenantID == "" {
		return nil, errors.New("generate due: tenant id required")
	}
	today := time.Now().UTC().Format("2006-01-02")
	rows, err := gen.New(r.db).ListDueTemplatesForTenant(ctx, gen.ListDueTemplatesForTenantParams{
		TenantID: tenantID,
		NextDue:  today,
	})
	if err != nil {
		return nil, fmt.Errorf("generate due: list: %w", err)
	}
	out := make([]GeneratedInvoice, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		inv, e := r.GenerateOne(ctx, rows[i].TenantID, rows[i].ID)
		if e != nil {
			return nil, fmt.Errorf("generate due: template %s: %w", rows[i].ID, e)
		}
		if inv == nil {
			continue
		}
		out = append(out, GeneratedInvoice{
			TemplateID: rows[i].ID, InvoiceID: inv.ID, InvoiceNumber: inv.Number,
		})
	}
	return out, nil
}
