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

// recurringListSelect mirrors the ListRecurringTemplates sqlc query body up to
// the WHERE. Keep in sync with internal/db/queries/recurring_templates.sql.
// tenant_id is the only bound parameter before the listquery clause args.
const recurringListSelect = `SELECT r.*, p.name AS participant_name, p.uuid AS participant_uuid, pm.uuid AS plan_manager_uuid
FROM recurring_templates r
LEFT JOIN participants p ON r.participant_id = p.id AND p.tenant_id = r.tenant_id
LEFT JOIN plan_managers pm ON r.plan_manager_id = pm.id AND pm.tenant_id = r.tenant_id
WHERE r.tenant_id = ?`

// RecurringCols is the listquery allowlist for recurring templates. Keys match
// the JSON field names so the frontend column key drives filter, sort, and
// display with one identifier.
var RecurringCols = listquery.Spec{
	"name":            {Col: "r.name", Filter: listquery.Text},
	"participantName": {Col: "p.name", Filter: listquery.Text},
	"frequency":       {Col: "r.frequency", Filter: listquery.Enum},
	"nextDue":         {Col: "r.next_due", Filter: listquery.Date},
	"isActive":        {Col: "r.is_active", Filter: listquery.Enum},
	"taxRate":         {Col: "r.tax_rate", Filter: listquery.Number},
}

// RecurringTemplate is the domain view of a recurring invoice template. Line
// items are stored as a JSON string column and unmarshalled into the slice. The
// public identifier is the uuid (json "id"); the int PK is internal-only. The
// participant/plan-manager FKs are exposed as the related entities' uuids (json
// "participantId"/"planManagerId"), resolved via LEFT JOIN, never the int FKs.
// participantID is the internal participant int PK, retained for generation
// snapshots (not serialized).
type RecurringTemplate struct {
	ID              int64            `json:"-"`
	UUID            string           `json:"id"`
	participantID   *int64           // internal participant FK, used for generation snapshots
	ParticipantUUID *string          `json:"participantId"`
	ParticipantName string           `json:"participantName"`
	PlanManagerID   *int64           `json:"-"`
	PlanManagerUUID *string          `json:"planManagerId"`
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
	SupportItemID *string `json:"supportItemId"` // control-DB support_items.uuid
	CustomItemID  *string `json:"customItemId"`  // tenant custom_items.uuid
	Code          string  `json:"code"`
	Description   string  `json:"description"`
	Unit          string  `json:"unit"`
	Quantity      float64 `json:"quantity"`
	UnitPrice     float64 `json:"unitPrice"`
	GstFree       bool    `json:"gstFree"`
	SortOrder     int64   `json:"sortOrder"`
}

// RecurringInput is the writable subset of a recurring template. Participant and
// plan-manager arrive as uuid strings (the public identifiers) and are resolved
// to the int FKs before insert/update. ParticipantUUID is required; an empty/nil
// PlanManagerUUID maps to a NULL FK.
type RecurringInput struct {
	ParticipantUUID *string         `json:"participantId"`
	PlanManagerUUID *string         `json:"planManagerId"`
	Name            string          `json:"name"`
	Frequency       string          `json:"frequency"`
	NextDue         string          `json:"nextDue"`
	LineItems       []RecurringLine `json:"lineItems"`
	TaxRate         float64         `json:"taxRate"`
	Notes           string          `json:"notes"`
	IsActive        bool            `json:"isActive"`
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
	db   db.Executor
	snap *billing.SnapshotBuilder
}

// NewRepo constructs a repository. A nil db is a programmer error.
func NewRepo(db db.Executor) *Repo {
	if db == nil {
		panic("recurring: NewRepo requires a non-nil *sql.DB")
	}
	return &Repo{db: db, snap: billing.NewSnapshotBuilder(db)}
}

// validFrequencies is the closed set of supported cadences.
var validFrequencies = map[string]bool{"weekly": true, "monthly": true, "quarterly": true}

// errParticipantNotFound / errPlanManagerNotFound are the sentinels returned when
// an inbound uuid does not resolve to a row in the tenant. Handlers map them to a
// 400 validation error.
var (
	errParticipantNotFound = errors.New("recurring: participant not found")
	errPlanManagerNotFound = errors.New("recurring: plan manager not found")
)

// validate checks a writable template input at the module boundary.
func (r *Repo) validate(in RecurringInput) error {
	if in.Name == "" {
		return errors.New("recurring: name is required")
	}
	if in.ParticipantUUID == nil || *in.ParticipantUUID == "" {
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

// resolveParticipant translates the required inbound participant uuid into the int
// FK for insert/update. An unknown uuid (foreign or absent) → errParticipantNotFound.
func (r *Repo) resolveParticipant(ctx context.Context, q *gen.Queries, tenantID int64, pUUID *string) (sql.NullInt64, error) {
	if pUUID == nil || *pUUID == "" {
		return sql.NullInt64{}, errParticipantNotFound
	}
	id, err := q.GetParticipantIDByUUID(ctx, gen.GetParticipantIDByUUIDParams{TenantID: tenantID, Uuid: *pUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, errParticipantNotFound
	}
	if err != nil {
		return sql.NullInt64{}, fmt.Errorf("resolve participant: %w", err)
	}
	return sql.NullInt64{Int64: id, Valid: true}, nil
}

// resolvePlanManager translates an optional inbound plan-manager uuid into the int
// FK. A nil/empty uuid → NULL FK. An unknown uuid → errPlanManagerNotFound.
func (r *Repo) resolvePlanManager(ctx context.Context, q *gen.Queries, tenantID int64, pmUUID *string) (sql.NullInt64, error) {
	if pmUUID == nil || *pmUUID == "" {
		return sql.NullInt64{}, nil
	}
	id, err := q.GetPlanManagerIDByUUID(ctx, gen.GetPlanManagerIDByUUIDParams{TenantID: tenantID, Uuid: *pmUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, errPlanManagerNotFound
	}
	if err != nil {
		return sql.NullInt64{}, fmt.Errorf("resolve plan manager: %w", err)
	}
	return sql.NullInt64{Int64: id, Valid: true}, nil
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

// Query returns one page of templates plus the total row count for the filter
// (ignoring pagination). The clause is built by listquery from an allowlisted
// spec, so its Where/Order fragments are injection-safe. Default order is by
// next_due ascending.
func (r *Repo) Query(ctx context.Context, tenantID int64, c listquery.Clause) ([]*RecurringTemplate, int64, error) {
	if tenantID == 0 {
		return nil, 0, errors.New("query recurring: tenant id required")
	}
	var total int64
	countSQL := "SELECT count(*) FROM (" + recurringListSelect + c.Where + ")"
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count recurring: %w", err)
	}
	order := c.Order
	if order == "" {
		order = " ORDER BY r.next_due"
	}
	sqlText := recurringListSelect + c.Where + order + c.Limit
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query recurring: %w", err)
	}
	defer rows.Close()
	out := make([]*RecurringTemplate, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var i gen.ListRecurringTemplatesRow
		if err := rows.Scan(&i.ID, &i.Uuid, &i.TenantID, &i.ParticipantID, &i.PlanManagerID,
			&i.Name, &i.Frequency, &i.NextDue, &i.LineItems, &i.TaxRate, &i.Notes,
			&i.IsActive, &i.CreatedAt, &i.UpdatedAt, &i.ParticipantName, &i.ParticipantUuid, &i.PlanManagerUuid); err != nil {
			return nil, 0, fmt.Errorf("scan recurring: %w", err)
		}
		out = append(out, listRowToTemplate(i))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query recurring: %w", err)
	}
	return out, total, nil
}

// Get returns the template by uuid (with participant/plan-manager uuids, name,
// and line items), or (nil, nil) when absent.
func (r *Repo) Get(ctx context.Context, tenantID int64, uuid string) (*RecurringTemplate, error) {
	row, err := gen.New(r.db).GetRecurringTemplate(ctx, gen.GetRecurringTemplateParams{TenantID: tenantID, Uuid: uuid})
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
	var newUUID string
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		pid, e := r.resolveParticipant(ctx, q, tenantID, in.ParticipantUUID)
		if e != nil {
			return e
		}
		pmID, e := r.resolvePlanManager(ctx, q, tenantID, in.PlanManagerUUID)
		if e != nil {
			return e
		}
		tpl, e := q.CreateRecurringTemplate(ctx, gen.CreateRecurringTemplateParams{
			Uuid:          uuid.NewString(),
			TenantID:      tenantID,
			ParticipantID: pid,
			PlanManagerID: pmID,
			Name:          in.Name,
			Frequency:     in.Frequency,
			NextDue:       in.NextDue,
			LineItems:     lineItemsJSON,
			TaxRate:       in.TaxRate,
			Notes:         in.Notes,
			IsActive:      db.B2i(in.IsActive),
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newUUID = tpl.Uuid
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "recurring_template", EntityID: tpl.ID, Action: "create",
		})
	})
	if errors.Is(err, errParticipantNotFound) {
		return nil, errParticipantNotFound
	}
	if errors.Is(err, errPlanManagerNotFound) {
		return nil, errPlanManagerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("create recurring: %w", err)
	}
	return r.Get(ctx, tenantID, newUUID)
}

// Update validates and rewrites a template by uuid, atomically with one audit
// row. Returns (nil, nil) when the template does not exist. The audit entry
// records the row's int PK, resolved by-uuid inside the same tx.
func (r *Repo) Update(ctx context.Context, tenantID int64, uuid string, in RecurringInput) (*RecurringTemplate, error) {
	if err := r.validate(in); err != nil {
		return nil, err
	}
	lineItemsJSON, err := marshalLines(in.LineItems)
	if err != nil {
		return nil, fmt.Errorf("update recurring: %w", err)
	}
	var missing bool
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		existing, e := q.GetRecurringTemplate(ctx, gen.GetRecurringTemplateParams{TenantID: tenantID, Uuid: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("load existing: %w", e)
		}
		pid, e := r.resolveParticipant(ctx, q, tenantID, in.ParticipantUUID)
		if e != nil {
			return e
		}
		pmID, e := r.resolvePlanManager(ctx, q, tenantID, in.PlanManagerUUID)
		if e != nil {
			return e
		}
		if _, e := q.UpdateRecurringTemplate(ctx, gen.UpdateRecurringTemplateParams{
			ParticipantID: pid,
			PlanManagerID: pmID,
			Name:          in.Name,
			Frequency:     in.Frequency,
			NextDue:       in.NextDue,
			LineItems:     lineItemsJSON,
			TaxRate:       in.TaxRate,
			Notes:         in.Notes,
			IsActive:      db.B2i(in.IsActive),
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
			TenantID:      tenantID,
			Uuid:          uuid,
		}); e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "recurring_template", EntityID: existing.ID, Action: "update",
		})
	})
	if missing {
		return nil, nil
	}
	if errors.Is(err, errParticipantNotFound) {
		return nil, errParticipantNotFound
	}
	if errors.Is(err, errPlanManagerNotFound) {
		return nil, errPlanManagerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update recurring: %w", err)
	}
	return r.Get(ctx, tenantID, uuid)
}

// Delete removes a template by uuid and writes one audit row, atomically. The
// audit entry records the row's int PK, resolved by-uuid in the same tx. A
// missing row is a silent no-op.
func (r *Repo) Delete(ctx context.Context, tenantID int64, uuid string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetRecurringTemplate(ctx, gen.GetRecurringTemplateParams{TenantID: tenantID, Uuid: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return fmt.Errorf("lookup: %w", e)
		}
		if e := q.DeleteRecurringTemplate(ctx, gen.DeleteRecurringTemplateParams{TenantID: tenantID, Uuid: uuid}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "recurring_template", EntityID: row.ID, Action: "delete",
		})
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

// GenerateOne creates a draft invoice from the template (by uuid) AND advances
// next_due in the same transaction (idempotent re-runs). Returns (nil, nil) when
// the template is missing. The returned invoice is re-read after commit.
func (r *Repo) GenerateOne(ctx context.Context, tenantID int64, templateUUID string) (*invoice.Invoice, error) {
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
	snaps := r.buildGenSnapshots(ctx, tenantID, tpl.participantID, tpl.PlanManagerID)

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
	if tpl.participantID != nil {
		pid = *tpl.participantID
	}
	t := billing.ComputeTotals(items, tax)
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		Uuid:             uuid.NewString(),
		TenantID:         tenantID,
		Number:           num,
		ParticipantID:    pid,
		PlanManagerID:    db.NullID(tpl.PlanManagerID),
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
		inv, e := r.GenerateOne(ctx, rows[i].TenantID, rows[i].Uuid)
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
		ID: r.ID, UUID: r.Uuid, participantID: db.PtrID(r.ParticipantID),
		ParticipantUUID: db.PtrStr(r.ParticipantUuid), ParticipantName: r.ParticipantName.String,
		PlanManagerID: db.PtrID(r.PlanManagerID), PlanManagerUUID: db.PtrStr(r.PlanManagerUuid),
		Name: r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func activeRowToTemplate(r gen.ListActiveRecurringTemplatesRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, UUID: r.Uuid, participantID: db.PtrID(r.ParticipantID),
		ParticipantUUID: db.PtrStr(r.ParticipantUuid), ParticipantName: r.ParticipantName.String,
		PlanManagerID: db.PtrID(r.PlanManagerID), PlanManagerUUID: db.PtrStr(r.PlanManagerUuid),
		Name: r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func getRowToTemplate(r gen.GetRecurringTemplateRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, UUID: r.Uuid, participantID: db.PtrID(r.ParticipantID),
		ParticipantUUID: db.PtrStr(r.ParticipantUuid), ParticipantName: r.ParticipantName.String,
		PlanManagerID: db.PtrID(r.PlanManagerID), PlanManagerUUID: db.PtrStr(r.PlanManagerUuid),
		Name: r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
