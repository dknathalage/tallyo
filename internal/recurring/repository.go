package recurring

// NOTE (J4): rewritten for the tenant-scoped NDIS recurring_templates schema.
// Templates carry participant_id / plan_manager_id and a JSON line_items column.
// The stored line shape is NDIS-aware (code, serviceDate, unit, unitPrice,
// gstFree). tax_rate is a stored percentage on the template; generation computes
// the tax amount from it. NDIS price-cap / plan-window validation is J10.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/numbering"
	"github.com/google/uuid"
)

// RecurringTemplate is the domain view of a recurring invoice template. Line
// items are stored as a JSON string column and unmarshalled into the slice.
type RecurringTemplate struct {
	ID              int64            `json:"id"`
	UUID            string           `json:"uuid"`
	ParticipantID   *int64           `json:"participantId"`
	ParticipantName string           `json:"participantName"`
	PlanManagerID   *int64           `json:"planManagerId"`
	Name            string           `json:"name"`
	Frequency       string           `json:"frequency"`
	NextDue         string           `json:"nextDue"`
	LineItems       []*RecurringLine `json:"lineItems"`
	TaxRate         float64          `json:"taxRate"`
	Notes           string           `json:"notes"`
	IsActive        bool             `json:"isActive"`
	CreatedAt       string           `json:"createdAt"`
	UpdatedAt       string           `json:"updatedAt"`
}

// RecurringLine is one line in a template's stored line_items JSON.
type RecurringLine struct {
	SupportItemID *int64  `json:"supportItemId"`
	CustomItemID  *int64  `json:"customItemId"`
	Code          string  `json:"code"`
	Description   string  `json:"description"`
	Unit          string  `json:"unit"`
	Quantity      float64 `json:"quantity"`
	UnitPrice     float64 `json:"unitPrice"`
	GstFree       bool    `json:"gstFree"`
	SortOrder     int64   `json:"sortOrder"`
}

// RecurringInput is the writable subset of a recurring template.
type RecurringInput struct {
	ParticipantID *int64          `json:"participantId"`
	PlanManagerID *int64          `json:"planManagerId"`
	Name          string          `json:"name"`
	Frequency     string          `json:"frequency"`
	NextDue       string          `json:"nextDue"`
	LineItems     []RecurringLine `json:"lineItems"`
	TaxRate       float64         `json:"taxRate"`
	Notes         string          `json:"notes"`
	IsActive      bool            `json:"isActive"`
}

// GeneratedInvoice identifies an invoice produced by the due sweep.
type GeneratedInvoice struct {
	TemplateID    int64  `json:"templateId"`
	InvoiceID     int64  `json:"invoiceId"`
	InvoiceNumber string `json:"invoiceNumber"`
}

// Repo reads and writes recurring_templates (tenant-scoped) and
// generates invoices from them. Generation advances next_due in the same
// transaction as the invoice insert so re-running the due sweep never
// double-generates.
type Repo struct {
	db   *sql.DB
	snap *billing.SnapshotBuilder
}

// NewRepo constructs a repository. A nil db is a programmer error.
func NewRepo(db *sql.DB) *Repo {
	if db == nil {
		panic("recurring: NewRepo requires a non-nil *sql.DB")
	}
	return &Repo{db: db, snap: billing.NewSnapshotBuilder(db)}
}

// validFrequencies is the closed set of supported cadences.
var validFrequencies = map[string]bool{"weekly": true, "monthly": true, "quarterly": true}

// validate checks a writable template input at the module boundary.
func (r *Repo) validate(in RecurringInput) error {
	if in.Name == "" {
		return errors.New("recurring: name is required")
	}
	if in.ParticipantID == nil || *in.ParticipantID == 0 {
		return errors.New("recurring: participant required")
	}
	if !validFrequencies[in.Frequency] {
		return errors.New("recurring: invalid frequency")
	}
	if in.NextDue == "" {
		return errors.New("recurring: next due is required")
	}
	return nil
}

// List returns templates (all, or active only), each with participant name and
// parsed line items. The slice is always non-nil.
func (r *Repo) List(ctx context.Context, tenantID int64, activeOnly bool) ([]*RecurringTemplate, error) {
	q := gen.New(r.db)
	if activeOnly {
		rows, err := q.ListActiveRecurringTemplates(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list active recurring: %w", err)
		}
		out := make([]*RecurringTemplate, 0, len(rows))
		for i := range rows { // bounded by len(rows)
			out = append(out, activeRowToTemplate(rows[i]))
		}
		return out, nil
	}
	rows, err := q.ListRecurringTemplates(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list recurring: %w", err)
	}
	out := make([]*RecurringTemplate, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, listRowToTemplate(rows[i]))
	}
	return out, nil
}

// Get returns the template (with participant name and line items), or (nil, nil)
// when absent.
func (r *Repo) Get(ctx context.Context, tenantID, id int64) (*RecurringTemplate, error) {
	row, err := gen.New(r.db).GetRecurringTemplate(ctx, gen.GetRecurringTemplateParams{TenantID: tenantID, ID: id})
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
func (r *Repo) Create(ctx context.Context, tenantID int64, in RecurringInput) (*RecurringTemplate, error) {
	if tenantID == 0 {
		return nil, errors.New("create recurring: tenant id required")
	}
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
			Uuid:          uuid.NewString(),
			TenantID:      tenantID,
			ParticipantID: nullID(in.ParticipantID),
			PlanManagerID: nullID(in.PlanManagerID),
			Name:          in.Name,
			Frequency:     in.Frequency,
			NextDue:       in.NextDue,
			LineItems:     lineItemsJSON,
			TaxRate:       in.TaxRate,
			Notes:         in.Notes,
			IsActive:      b2i(in.IsActive),
			CreatedAt:     now,
			UpdatedAt:     now,
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
	return r.Get(ctx, tenantID, newID)
}

// Update validates and rewrites a template, atomically with one audit row.
// Returns (nil, nil) when the template does not exist.
func (r *Repo) Update(ctx context.Context, tenantID, id int64, in RecurringInput) (*RecurringTemplate, error) {
	if err := r.validate(in); err != nil {
		return nil, err
	}
	if _, err := gen.New(r.db).GetRecurringTemplate(ctx, gen.GetRecurringTemplateParams{TenantID: tenantID, ID: id}); errors.Is(err, sql.ErrNoRows) {
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
			ParticipantID: nullID(in.ParticipantID),
			PlanManagerID: nullID(in.PlanManagerID),
			Name:          in.Name,
			Frequency:     in.Frequency,
			NextDue:       in.NextDue,
			LineItems:     lineItemsJSON,
			TaxRate:       in.TaxRate,
			Notes:         in.Notes,
			IsActive:      b2i(in.IsActive),
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
			TenantID:      tenantID,
			ID:            id,
		})
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("update recurring: %w", err)
	}
	return r.Get(ctx, tenantID, id)
}

// Delete removes a template and writes one audit row.
func (r *Repo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "recurring_template", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).DeleteRecurringTemplate(ctx, gen.DeleteRecurringTemplateParams{TenantID: tenantID, ID: id}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return nil
	})
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

// GenerateOne creates a draft invoice from the template AND advances next_due in
// the same transaction (idempotent re-runs). Returns (nil, nil) when the
// template is missing. The returned invoice is re-read after commit.
func (r *Repo) GenerateOne(ctx context.Context, tenantID, templateID int64) (*invoice.Invoice, error) {
	tpl, err := r.Get(ctx, tenantID, templateID)
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
	snaps := r.buildGenSnapshots(ctx, tenantID, tpl.ParticipantID, tpl.PlanManagerID)

	var invID int64
	err = numbering.WithRetry(ctx, 10, func() error {
		return r.generateTx(ctx, tenantID, tpl, items, today, tax, snaps, &invID)
	})
	if err != nil {
		return nil, fmt.Errorf("generate one: %w", err)
	}
	return invoice.NewInvoices(r.db).Get(ctx, tenantID, invID)
}

// genSnapshots holds the default snapshot JSON for a generated invoice.
type genSnapshots struct {
	business string
	client   string
	payer    string
}

// buildGenSnapshots builds default snapshots for a generated invoice.
func (r *Repo) buildGenSnapshots(ctx context.Context, tenantID int64, participantID, planManagerID *int64) genSnapshots {
	var pid int64
	if participantID != nil {
		pid = *participantID
	}
	return genSnapshots{
		business: r.snap.Business(ctx, tenantID),
		client:   r.snap.Participant(ctx, tenantID, pid),
		payer:    r.snap.PlanManager(ctx, tenantID, planManagerID),
	}
}

// generateTx runs one generation attempt in one transaction for idempotency.
func (r *Repo) generateTx(ctx context.Context, tenantID int64, tpl *RecurringTemplate, items []billing.LineItemInput,
	today string, tax float64, snaps genSnapshots, invID *int64) error {
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
	inv, err := q.CreateInvoice(ctx, recurringInvoiceParams(tenantID, tpl, items, today, tax, snaps, num))
	if err != nil {
		return err
	}
	if err := invoice.InsertLineItems(ctx, q, tenantID, inv.ID, items); err != nil {
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
func recurringInvoiceParams(tenantID int64, tpl *RecurringTemplate, items []billing.LineItemInput, today string, tax float64, snaps genSnapshots, num string) gen.CreateInvoiceParams {
	var pid int64
	if tpl.ParticipantID != nil {
		pid = *tpl.ParticipantID
	}
	t := billing.ComputeTotals(items, tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		Uuid:             uuid.NewString(),
		TenantID:         tenantID,
		Number:           num,
		ParticipantID:    pid,
		PlanManagerID:    nullID(tpl.PlanManagerID),
		Status:           "draft",
		IssueDate:        today,
		DueDate:          today,
		Subtotal:         t.Subtotal,
		Tax:              t.Tax,
		Total:            t.Total,
		Notes:            nzMaybe(tpl.Notes),
		BusinessSnapshot: nzMaybe(snaps.business),
		ClientSnapshot:   nzMaybe(snaps.client),
		PayerSnapshot:    nzMaybe(snaps.payer),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// GenerateDueForTenant generates one invoice per due template of ONE tenant
// whose next_due has passed. Idempotent: each generation advances next_due in
// its own transaction. Returns a non-nil slice. This is the per-tenant sweep
// path (spec §8): the caller iterates active tenants and skips suspended ones.
func (r *Repo) GenerateDueForTenant(ctx context.Context, tenantID int64) ([]GeneratedInvoice, error) {
	if tenantID == 0 {
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
			return nil, fmt.Errorf("generate due: template %d: %w", rows[i].ID, e)
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
func parseLines(lines []*RecurringLine) []billing.LineItemInput {
	out := make([]billing.LineItemInput, 0, len(lines))
	for i := range lines { // bounded by len(lines)
		l := lines[i]
		out = append(out, billing.LineItemInput{
			SupportItemID: l.SupportItemID,
			CustomItemID:  l.CustomItemID,
			Code:          l.Code,
			Description:   l.Description,
			Unit:          l.Unit,
			Quantity:      l.Quantity,
			UnitPrice:     l.UnitPrice,
			GstFree:       l.GstFree,
			SortOrder:     l.SortOrder,
		})
	}
	return out
}

func listRowToTemplate(r gen.ListRecurringTemplatesRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, UUID: r.Uuid, ParticipantID: ptrID(r.ParticipantID), ParticipantName: r.ParticipantName.String,
		PlanManagerID: ptrID(r.PlanManagerID),
		Name:          r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func activeRowToTemplate(r gen.ListActiveRecurringTemplatesRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, UUID: r.Uuid, ParticipantID: ptrID(r.ParticipantID), ParticipantName: r.ParticipantName.String,
		PlanManagerID: ptrID(r.PlanManagerID),
		Name:          r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func getRowToTemplate(r gen.GetRecurringTemplateRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, UUID: r.Uuid, ParticipantID: ptrID(r.ParticipantID), ParticipantName: r.ParticipantName.String,
		PlanManagerID: ptrID(r.PlanManagerID),
		Name:          r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// b2i maps a bool to the int64 column convention (true -> 1, false -> 0).
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// nullID wraps an optional id into a sql.NullInt64 (invalid when nil).
func nullID(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

// ptrID unwraps a sql.NullInt64 into a *int64 (nil when invalid).
func ptrID(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// nzMaybe wraps a string into a sql.NullString that is invalid (SQL NULL) when
// the string is empty, and valid otherwise.
func nzMaybe(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
