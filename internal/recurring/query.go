package recurring

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/listquery"
)

// recurringListSelect mirrors the ListRecurringTemplates sqlc query body up to
// the WHERE. Keep in sync with internal/db/queries/recurring_templates.sql.
// tenant_id is the only bound parameter before the listquery clause args.
const recurringListSelect = `SELECT r.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM recurring_templates r
LEFT JOIN clients p ON r.client_id = p.id AND p.tenant_id = r.tenant_id
LEFT JOIN payers pm ON r.payer_id = pm.id AND pm.tenant_id = r.tenant_id
WHERE r.tenant_id = ?`

// RecurringCols is the listquery allowlist for recurring templates. Keys match
// the JSON field names so the frontend column key drives filter, sort, and
// display with one identifier.
var RecurringCols = listquery.Spec{
	"name":       {Col: "r.name", Filter: listquery.Text},
	"clientName": {Col: "p.name", Filter: listquery.Text},
	"frequency":  {Col: "r.frequency", Filter: listquery.Enum},
	"nextDue":    {Col: "r.next_due", Filter: listquery.Date},
	"isActive":   {Col: "r.is_active", Filter: listquery.Enum},
	"taxRate":    {Col: "r.tax_rate", Filter: listquery.Number},
}

// List returns templates (all, or active only), each with client name and
// parsed line items. The slice is always non-nil.
func (r *Repo) List(ctx context.Context, tenantID string, activeOnly bool) ([]*RecurringTemplate, error) {
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
func (r *Repo) Query(ctx context.Context, tenantID string, c listquery.Clause) ([]*RecurringTemplate, int64, error) {
	if tenantID == "" {
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
		if err := rows.Scan(&i.ID, &i.TenantID, &i.ClientID, &i.PayerID,
			&i.Name, &i.Frequency, &i.NextDue, &i.LineItems, &i.TaxRate, &i.Notes,
			&i.IsActive, &i.CreatedAt, &i.UpdatedAt, &i.ClientName, &i.ClientUuid, &i.PayerUuid); err != nil {
			return nil, 0, fmt.Errorf("scan recurring: %w", err)
		}
		out = append(out, listRowToTemplate(i))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query recurring: %w", err)
	}
	return out, total, nil
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
			ItemID:       l.ItemID,
			CustomItemID: l.CustomItemID,
			Code:         l.Code,
			Description:  l.Description,
			Unit:         l.Unit,
			Quantity:     l.Quantity,
			UnitPrice:    l.UnitPrice,
			Taxable:      l.Taxable,
			SortOrder:    l.SortOrder,
		})
	}
	return out
}

func listRowToTemplate(r gen.ListRecurringTemplatesRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, clientID: db.PtrStr(r.ClientID),
		ClientUUID: db.PtrStr(r.ClientUuid), ClientName: r.ClientName.String,
		PayerID: db.PtrStr(r.PayerID), PayerUUID: db.PtrStr(r.PayerUuid),
		Name: r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func activeRowToTemplate(r gen.ListActiveRecurringTemplatesRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, clientID: db.PtrStr(r.ClientID),
		ClientUUID: db.PtrStr(r.ClientUuid), ClientName: r.ClientName.String,
		PayerID: db.PtrStr(r.PayerID), PayerUUID: db.PtrStr(r.PayerUuid),
		Name: r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func getRowToTemplate(r gen.GetRecurringTemplateRow) *RecurringTemplate {
	return &RecurringTemplate{
		ID: r.ID, clientID: db.PtrStr(r.ClientID),
		ClientUUID: db.PtrStr(r.ClientUuid), ClientName: r.ClientName.String,
		PayerID: db.PtrStr(r.PayerID), PayerUUID: db.PtrStr(r.PayerUuid),
		Name: r.Name, Frequency: r.Frequency, NextDue: r.NextDue,
		LineItems: unmarshalLines(r.LineItems), TaxRate: r.TaxRate, Notes: r.Notes,
		IsActive: r.IsActive != 0, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// genNullStrPtr returns a *string for a non-empty NullString, else nil.
func genNullStrPtr(ns sql.NullString) *string {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	s := ns.String
	return &s
}
