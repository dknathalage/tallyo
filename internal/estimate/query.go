package estimate

// Read paths: the list/filter SQL (List/Query/ListByStatus/ListClientEstimates),
// the listquery allowlist + base SELECT, and the list-only row mappers. Split
// out of repository.go to keep that file to core CRUD.

import (
	"context"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/listquery"
)

// estimateListSelect mirrors the ListEstimates sqlc query body up to the WHERE.
// Keep in sync with internal/db/queries/estimates.sql. tenant_id is the only ?.
const estimateListSelect = `SELECT e.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid, ci.id AS converted_invoice_uuid
FROM estimates e
LEFT JOIN clients p ON e.client_id = p.id AND p.tenant_id = e.tenant_id
LEFT JOIN payers pm ON e.payer_id = pm.id AND pm.tenant_id = e.tenant_id
LEFT JOIN invoices ci ON e.converted_invoice_id = ci.id AND ci.tenant_id = e.tenant_id
WHERE e.tenant_id = ?`

// EstimateCols is the listquery allowlist for estimates. Keys match the JSON
// field names so one column key drives filter, sort and display.
var EstimateCols = listquery.Spec{
	"number":     {Col: "e.number", Filter: listquery.Text},
	"clientName": {Col: "p.name", Filter: listquery.Text},
	"status":     {Col: "e.status", Filter: listquery.Enum},
	"issueDate":  {Col: "e.issue_date", Filter: listquery.Date},
	"total":      {Col: "e.total", Filter: listquery.Number},
}

// List returns every estimate for the tenant (header only), newest first.
func (r *EstimatesRepo) List(ctx context.Context, tenantID string) ([]*Estimate, error) {
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
func (r *EstimatesRepo) Query(ctx context.Context, tenantID string, c listquery.Clause) ([]*Estimate, int64, error) {
	if tenantID == "" {
		return nil, 0, errors.New("query estimates: tenant id required")
	}
	var total int64
	countSQL := db.Rebind("SELECT count(*) FROM (" + estimateListSelect + c.Where + ") AS sub")
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count estimates: %w", err)
	}
	order := c.Order
	if order == "" {
		order = " ORDER BY e.created_at DESC"
	}
	sqlText := db.Rebind(estimateListSelect + c.Where + order + c.Limit)
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query estimates: %w", err)
	}
	defer rows.Close()
	out := make([]*Estimate, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var f estimateFields
		var tenant string
		if err := rows.Scan(&f.id, &tenant, &f.number, &f.clientID,
			&f.payerID, &f.status, &f.issueDate, &f.validUntil, &f.subtotal,
			&f.tax, &f.total, &f.notes, &f.convertedInvoiceID, &f.businessSnap,
			&f.clientSnap, &f.payerSnap, &f.createdAt, &f.updatedAt, &f.clientName,
			&f.clientUUID, &f.payerUUID, &f.convertedInvoiceUUID); err != nil {
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
func (r *EstimatesRepo) ListByStatus(ctx context.Context, tenantID string, status string) ([]*Estimate, error) {
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

// ListClientEstimates returns one client's estimates (header only).
func (r *EstimatesRepo) ListClientEstimates(ctx context.Context, tenantID, clientID string) ([]*Estimate, error) {
	rows, err := gen.New(r.db).ListClientEstimates(ctx, gen.ListClientEstimatesParams{
		TenantID: tenantID,
		ClientID: db.NullStr(&clientID),
	})
	if err != nil {
		return nil, fmt.Errorf("list client estimates: %w", err)
	}
	out := make([]*Estimate, 0, len(rows))
	for i := range rows {
		out = append(out, toEstimateFromRow(estimateFieldsFromClient(rows[i])))
	}
	return out, nil
}

func estimateFieldsFromList(r gen.ListEstimatesRow) estimateFields {
	return estimateFields{
		id: r.ID, number: r.Number, clientID: r.ClientID,
		clientUUID: r.ClientUuid, payerID: r.PayerID, payerUUID: r.PayerUuid,
		status: r.Status, issueDate: r.IssueDate, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		convertedInvoiceID: r.ConvertedInvoiceID, convertedInvoiceUUID: r.ConvertedInvoiceUuid,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

func estimateFieldsFromStatus(r gen.ListEstimatesByStatusRow) estimateFields {
	return estimateFields{
		id: r.ID, number: r.Number, clientID: r.ClientID,
		clientUUID: r.ClientUuid, payerID: r.PayerID, payerUUID: r.PayerUuid,
		status: r.Status, issueDate: r.IssueDate, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		convertedInvoiceID: r.ConvertedInvoiceID, convertedInvoiceUUID: r.ConvertedInvoiceUuid,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}

func estimateFieldsFromClient(r gen.ListClientEstimatesRow) estimateFields {
	return estimateFields{
		id: r.ID, number: r.Number, clientID: r.ClientID,
		clientUUID: r.ClientUuid, payerID: r.PayerID, payerUUID: r.PayerUuid,
		status: r.Status, issueDate: r.IssueDate, validUntil: r.ValidUntil,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		convertedInvoiceID: r.ConvertedInvoiceID, convertedInvoiceUUID: r.ConvertedInvoiceUuid,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
	}
}
