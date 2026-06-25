package invoice

// Read paths: the list/filter SQL (List/Query/ListByStatus/ListClientInvoices),
// the listquery allowlist + base SELECT, and the list-only row mappers. Split
// out of repository.go to keep that file to core CRUD.

import (
	"context"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/listquery"
)

// invoiceListSelect mirrors the ListInvoices sqlc query body up to the WHERE.
// Keep in sync with internal/db/queries/invoices.sql. The tenant filter is the
// FIRST and ONLY ? in the base; listquery's c.Where is appended as " AND ...".
const invoiceListSelect = `SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = ?`

// InvoiceCols is the listquery allowlist for invoices. Keys match the JSON field
// names so the frontend column key drives filter, sort and display with one
// identifier. Invoices are a read-only document list (no drawer edit).
var InvoiceCols = listquery.Spec{
	"number":     {Col: "i.number", Filter: listquery.Text},
	"clientName": {Col: "p.name", Filter: listquery.Text},
	"status":     {Col: "i.status", Filter: listquery.Enum},
	"issueDate":  {Col: "i.issue_date", Filter: listquery.Date},
	"total":      {Col: "i.total", Filter: listquery.Number},
}

// List returns every invoice for the tenant (header only), newest first.
func (r *InvoicesRepo) List(ctx context.Context, tenantID string) ([]*Invoice, error) {
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
func (r *InvoicesRepo) Query(ctx context.Context, tenantID string, c listquery.Clause) ([]*Invoice, int64, error) {
	if tenantID == "" {
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
		var tenant string
		if err := rows.Scan(&f.id, &tenant, &f.number, &f.clientID,
			&f.payerID, &f.status, &f.issueDate, &f.dueDate, &f.subtotal,
			&f.tax, &f.total, &f.notes, &f.businessSnap, &f.clientSnap, &f.payerSnap,
			&f.createdAt, &f.updatedAt, &f.clientName, &f.clientUUID, &f.payerUUID); err != nil {
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
func (r *InvoicesRepo) ListByStatus(ctx context.Context, tenantID string, status string) ([]*Invoice, error) {
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

// ListClientInvoices returns one client's invoices (header only).
func (r *InvoicesRepo) ListClientInvoices(ctx context.Context, tenantID, clientID string) ([]*Invoice, error) {
	rows, err := gen.New(r.db).ListClientInvoices(ctx, gen.ListClientInvoicesParams{
		TenantID: tenantID,
		ClientID: clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("list client invoices: %w", err)
	}
	out := make([]*Invoice, 0, len(rows))
	for i := range rows {
		out = append(out, toInvoiceFromRow(invoiceFieldsFromClient(rows[i])))
	}
	return out, nil
}

func invoiceFieldsFromList(r gen.ListInvoicesRow) invoiceFields {
	return invoiceFields{
		id: r.ID, number: r.Number, clientID: r.ClientID,
		payerID: r.PayerID,
		status:  r.Status, issueDate: r.IssueDate, dueDate: r.DueDate,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
		clientUUID: r.ClientUuid, payerUUID: r.PayerUuid,
	}
}

func invoiceFieldsFromStatus(r gen.ListInvoicesByStatusRow) invoiceFields {
	return invoiceFields{
		id: r.ID, number: r.Number, clientID: r.ClientID,
		payerID: r.PayerID,
		status:  r.Status, issueDate: r.IssueDate, dueDate: r.DueDate,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
		clientUUID: r.ClientUuid, payerUUID: r.PayerUuid,
	}
}

func invoiceFieldsFromClient(r gen.ListClientInvoicesRow) invoiceFields {
	return invoiceFields{
		id: r.ID, number: r.Number, clientID: r.ClientID,
		payerID: r.PayerID,
		status:  r.Status, issueDate: r.IssueDate, dueDate: r.DueDate,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
		clientUUID: r.ClientUuid, payerUUID: r.PayerUuid,
	}
}
