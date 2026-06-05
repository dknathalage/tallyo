package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/numbering"
	"github.com/google/uuid"
)

// RecurringTemplate is the domain view of a recurring invoice template. Line
// items are stored as a JSON string column and unmarshalled into the slice.
type RecurringTemplate struct {
	ID         int64            `json:"id"`
	UUID       string           `json:"uuid"`
	ClientID   *int64           `json:"clientId"`
	ClientName string           `json:"clientName"`
	Name       string           `json:"name"`
	Frequency  string           `json:"frequency"`
	NextDue    string           `json:"nextDue"`
	LineItems  []*RecurringLine `json:"lineItems"`
	TaxRate    float64          `json:"taxRate"`
	Notes      string           `json:"notes"`
	IsActive   bool             `json:"isActive"`
	CreatedAt  string           `json:"createdAt"`
	UpdatedAt  string           `json:"updatedAt"`
}

// RecurringLine is one line in a template's stored line_items JSON.
type RecurringLine struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	Rate        float64 `json:"rate"`
	Notes       string  `json:"notes"`
	SortOrder   int64   `json:"sortOrder"`
}

// RecurringInput is the writable subset of a recurring template.
type RecurringInput struct {
	ClientID  *int64          `json:"clientId"`
	Name      string          `json:"name"`
	Frequency string          `json:"frequency"`
	NextDue   string          `json:"nextDue"`
	LineItems []RecurringLine `json:"lineItems"`
	TaxRate   float64         `json:"taxRate"`
	Notes     string          `json:"notes"`
	IsActive  bool            `json:"isActive"`
}

// GeneratedInvoice identifies an invoice produced by the due sweep.
type GeneratedInvoice struct {
	TemplateID    int64  `json:"templateId"`
	InvoiceID     int64  `json:"invoiceId"`
	InvoiceNumber string `json:"invoiceNumber"`
}

// RecurringRepo reads and writes recurring_templates and generates invoices from
// them. Generation advances next_due in the same transaction as the invoice
// insert so re-running the due sweep never double-generates.
type RecurringRepo struct {
	db   *sql.DB
	snap *InvoicesRepo // reused for the shared snapshot builders
}

// NewRecurring constructs a repository. A nil db is a programmer error.
func NewRecurring(db *sql.DB) *RecurringRepo {
	if db == nil {
		panic("repository: NewRecurring requires a non-nil *sql.DB")
	}
	return &RecurringRepo{db: db, snap: NewInvoices(db)}
}

// validFrequencies is the closed set of supported cadences.
var validFrequencies = map[string]bool{"weekly": true, "monthly": true, "quarterly": true}

// validate checks a writable template input at the module boundary.
func (r *RecurringRepo) validate(in RecurringInput) error {
	if in.Name == "" {
		return errors.New("recurring: name is required")
	}
	if in.ClientID == nil || *in.ClientID == 0 {
		return errors.New("recurring: client required")
	}
	if !validFrequencies[in.Frequency] {
		return errors.New("recurring: invalid frequency")
	}
	if in.NextDue == "" {
		return errors.New("recurring: next due is required")
	}
	return nil
}

// List returns templates (all, or active only), each with client name and
// parsed line items. The slice is always non-nil.
func (r *RecurringRepo) List(ctx context.Context, activeOnly bool) ([]*RecurringTemplate, error) {
	q := gen.New(r.db)
	if activeOnly {
		rows, err := q.ListActiveRecurringTemplates(ctx)
		if err != nil {
			return nil, fmt.Errorf("list active recurring: %w", err)
		}
		out := make([]*RecurringTemplate, 0, len(rows))
		for i := range rows { // bounded by len(rows)
			out = append(out, activeRowToTemplate(rows[i]))
		}
		return out, nil
	}
	rows, err := q.ListRecurringTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list recurring: %w", err)
	}
	out := make([]*RecurringTemplate, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, listRowToTemplate(rows[i]))
	}
	return out, nil
}

// Get returns the template (with client name and line items), or (nil, nil)
// when absent.
func (r *RecurringRepo) Get(ctx context.Context, id int64) (*RecurringTemplate, error) {
	row, err := gen.New(r.db).GetRecurringTemplate(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get recurring: %w", err)
	}
	return getRowToTemplate(row), nil
}

// Create validates and inserts a template, auditing the create with the real id,
// then re-reads the row.
func (r *RecurringRepo) Create(ctx context.Context, in RecurringInput) (*RecurringTemplate, error) {
	if err := r.validate(in); err != nil {
		return nil, err
	}
	lineItemsJSON, err := marshalLines(in.LineItems)
	if err != nil {
		return nil, fmt.Errorf("create recurring: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var newID int64
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		tpl, e := gen.New(tx).CreateRecurringTemplate(ctx, gen.CreateRecurringTemplateParams{
			Uuid:      uuid.NewString(),
			ClientID:  nullID(in.ClientID),
			Name:      in.Name,
			Frequency: in.Frequency,
			NextDue:   in.NextDue,
			LineItems: lineItemsJSON,
			TaxRate:   in.TaxRate,
			Notes:     in.Notes,
			IsActive:  boolToInt(in.IsActive),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newID = tpl.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "recurring_template", EntityID: tpl.ID, Action: "create",
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create recurring: %w", err)
	}
	return r.Get(ctx, newID)
}

// Update validates and rewrites a template, atomically with one audit row.
// Returns (nil, nil) when the template does not exist.
func (r *RecurringRepo) Update(ctx context.Context, id int64, in RecurringInput) (*RecurringTemplate, error) {
	if err := r.validate(in); err != nil {
		return nil, err
	}
	if _, err := gen.New(r.db).GetRecurringTemplate(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("update recurring: load existing: %w", err)
	}
	lineItemsJSON, err := marshalLines(in.LineItems)
	if err != nil {
		return nil, fmt.Errorf("update recurring: %w", err)
	}
	err = audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "recurring_template", EntityID: id, Action: "update",
	}, func(tx *sql.Tx) error {
		_, e := gen.New(tx).UpdateRecurringTemplate(ctx, gen.UpdateRecurringTemplateParams{
			ClientID:  nullID(in.ClientID),
			Name:      in.Name,
			Frequency: in.Frequency,
			NextDue:   in.NextDue,
			LineItems: lineItemsJSON,
			TaxRate:   in.TaxRate,
			Notes:     in.Notes,
			IsActive:  boolToInt(in.IsActive),
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			ID:        id,
		})
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("update recurring: %w", err)
	}
	return r.Get(ctx, id)
}

// Delete removes a template and writes one audit row.
func (r *RecurringRepo) Delete(ctx context.Context, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "recurring_template", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).DeleteRecurringTemplate(ctx, id); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return nil
	})
}

// AdvanceDate returns date advanced by one period of freq, in YYYY-MM-DD.
func (r *RecurringRepo) AdvanceDate(date, freq string) (string, error) {
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

// GenerateOne creates a draft invoice from the template AND advances next_due in
// the same transaction (idempotent re-runs). Returns (nil, nil) when the
// template is missing. The returned invoice is re-read after commit.
func (r *RecurringRepo) GenerateOne(ctx context.Context, templateID int64) (*Invoice, error) {
	tpl, err := r.Get(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("generate one: %w", err)
	}
	if tpl == nil {
		return nil, nil
	}
	items := parseLines(tpl.LineItems)
	today := time.Now().UTC().Format("2006-01-02")
	t := computeTotals(items, tpl.TaxRate)
	snaps := r.buildGenSnapshots(ctx, tpl.ClientID)

	var invID int64
	err = numbering.WithRetry(ctx, 10, func() error {
		return r.generateTx(ctx, tpl, items, today, t, snaps, &invID)
	})
	if err != nil {
		return nil, fmt.Errorf("generate one: %w", err)
	}
	return NewInvoices(r.db).Get(ctx, invID)
}

// genSnapshots holds the default snapshot JSON for a generated invoice.
type genSnapshots struct {
	business string
	client   string
	payer    string
}

// buildGenSnapshots builds default snapshots; payer is "{}" for recurring.
func (r *RecurringRepo) buildGenSnapshots(ctx context.Context, clientID *int64) genSnapshots {
	var cid int64
	if clientID != nil {
		cid = *clientID
	}
	return genSnapshots{
		business: r.snap.buildBusinessSnapshot(ctx),
		client:   r.snap.buildClientSnapshot(ctx, cid),
		payer:    "{}",
	}
}

// generateTx runs one generation attempt: number the invoice, insert header +
// items, advance next_due, and audit — all in one transaction for idempotency.
func (r *RecurringRepo) generateTx(ctx context.Context, tpl *RecurringTemplate, items []LineItemInput,
	today string, t totals, snaps genSnapshots, invID *int64) error {
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
	inv, err := q.CreateInvoice(ctx, recurringInvoiceParams(tpl, today, t, snaps, num))
	if err != nil {
		return err
	}
	if err := insertItems(ctx, q, inv.ID, items); err != nil {
		return err
	}
	newDue, err := r.AdvanceDate(tpl.NextDue, tpl.Frequency)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := q.SetRecurringNextDue(ctx, gen.SetRecurringNextDueParams{
		NextDue: newDue, UpdatedAt: now, ID: tpl.ID,
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
// params: a draft invoice dated today with custom terms and default snapshots.
func recurringInvoiceParams(tpl *RecurringTemplate, today string, t totals, snaps genSnapshots, num string) gen.CreateInvoiceParams {
	var cid int64
	if tpl.ClientID != nil {
		cid = *tpl.ClientID
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		Uuid:             uuid.NewString(),
		InvoiceNumber:    num,
		ClientID:         cid,
		Date:             today,
		DueDate:          today,
		PaymentTerms:     nz("custom"),
		Subtotal:         sql.NullFloat64{Float64: t.subtotal, Valid: true},
		TaxRate:          sql.NullFloat64{Float64: tpl.TaxRate, Valid: true},
		TaxRateID:        sql.NullInt64{},
		TaxAmount:        sql.NullFloat64{Float64: t.taxAmount, Valid: true},
		Total:            sql.NullFloat64{Float64: t.total, Valid: true},
		Notes:            nz(tpl.Notes),
		Status:           nz("draft"),
		CurrencyCode:     nz("USD"),
		BusinessSnapshot: nz(snaps.business),
		ClientSnapshot:   nz(snaps.client),
		PayerSnapshot:    nz(snaps.payer),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// GenerateDue generates one invoice per template whose next_due has passed.
// Idempotent: each generation advances next_due in its own transaction, so a
// re-run finds nothing due. Returns a non-nil slice.
func (r *RecurringRepo) GenerateDue(ctx context.Context) ([]GeneratedInvoice, error) {
	today := time.Now().UTC().Format("2006-01-02")
	rows, err := gen.New(r.db).ListDueTemplates(ctx, today)
	if err != nil {
		return nil, fmt.Errorf("generate due: list: %w", err)
	}
	out := make([]GeneratedInvoice, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		inv, e := r.GenerateOne(ctx, rows[i].ID)
		if e != nil {
			return nil, fmt.Errorf("generate due: template %d: %w", rows[i].ID, e)
		}
		if inv == nil {
			continue
		}
		out = append(out, GeneratedInvoice{
			TemplateID: rows[i].ID, InvoiceID: inv.ID, InvoiceNumber: inv.InvoiceNumber,
		})
	}
	return out, nil
}

// marshalLines serialises template line items to the stored JSON column.
func marshalLines(lines []RecurringLine) (string, error) {
	if lines == nil {
		lines = []RecurringLine{}
	}
	b, err := json.Marshal(lines)
	if err != nil {
		return "", fmt.Errorf("marshal line items: %w", err)
	}
	return string(b), nil
}

// unmarshalLines parses the stored JSON column; on parse failure it returns an
// empty (non-nil) slice rather than failing the read.
func unmarshalLines(s string) []*RecurringLine {
	out := []*RecurringLine{}
	if s == "" {
		return out
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return []*RecurringLine{}
	}
	return out
}

// parseLines converts stored template lines into writable line item inputs.
func parseLines(lines []*RecurringLine) []LineItemInput {
	out := make([]LineItemInput, 0, len(lines))
	for i := range lines { // bounded by len(lines)
		l := lines[i]
		out = append(out, LineItemInput{
			Description: l.Description,
			Quantity:    l.Quantity,
			Rate:        l.Rate,
			Notes:       l.Notes,
			SortOrder:   l.SortOrder,
		})
	}
	return out
}

// boolToInt maps a Go bool to the SQLite 0/1 integer convention.
func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func listRowToTemplate(r gen.ListRecurringTemplatesRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, UUID: r.Uuid, ClientID: ptrID(r.ClientID), ClientName: r.ClientName.String,
		Name: r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func activeRowToTemplate(r gen.ListActiveRecurringTemplatesRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, UUID: r.Uuid, ClientID: ptrID(r.ClientID), ClientName: r.ClientName.String,
		Name: r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func getRowToTemplate(r gen.GetRecurringTemplateRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, UUID: r.Uuid, ClientID: ptrID(r.ClientID), ClientName: r.ClientName.String,
		Name: r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
