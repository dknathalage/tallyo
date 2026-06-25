package invoice

// Row→domain mappers: the shared flat invoiceFields shape, the gen-row adapters
// (Get/GetByID here; the list adapters live in query.go), and the small helpers.
// Split out of repository.go to keep that file to core CRUD.

import (
	"database/sql"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// invoiceFields is the shared, flat shape of every invoices join row (List,
// ListByStatus, ListClientInvoices and Get all produce identical structs
// under distinct gen type names, each adding ClientName).
type invoiceFields struct {
	id                                  string
	number                              string
	clientID                            string
	payerID                             sql.NullString
	status, issueDate, dueDate          string
	subtotal, tax, total                float64
	notes                               sql.NullString
	businessSnap, clientSnap, payerSnap sql.NullString
	createdAt, updatedAt                string
	clientName                          sql.NullString
	clientUUID                          sql.NullString
	payerUUID                           sql.NullString
}

// toInvoiceFromRow builds a domain Invoice (without line items) from the
// unwrapped join columns. LineItems defaults to a non-nil empty slice.
func toInvoiceFromRow(f invoiceFields) *Invoice {
	return &Invoice{
		ID:               f.id,
		Number:           f.number,
		ClientID:         f.clientID,
		ClientUUID:       f.clientUUID.String,
		ClientName:       f.clientName.String,
		PayerUUID:        nullStrPtr(f.payerUUID),
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
		id: r.ID, number: r.Number, clientID: r.ClientID,
		payerID: r.PayerID,
		status:  r.Status, issueDate: r.IssueDate, dueDate: r.DueDate,
		subtotal: r.Subtotal, tax: r.Tax, total: r.Total, notes: r.Notes,
		businessSnap: r.BusinessSnapshot, clientSnap: r.ClientSnapshot, payerSnap: r.PayerSnapshot,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, clientName: r.ClientName,
		clientUUID: r.ClientUuid, payerUUID: r.PayerUuid,
	}
}

func invoiceFieldsFromGetByID(r gen.GetInvoiceByIDRow) invoiceFields {
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

// nullStrPtr returns a *string for a non-empty NullString, else nil.
func nullStrPtr(ns sql.NullString) *string {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	s := ns.String
	return &s
}

// mapLineItems maps generated joined line item rows to domain line items
// (non-nil); customItemId surfaces as the custom-item uuid.
func mapLineItems(rows []gen.ListLineItemsForInvoiceRow) []*billing.LineItem {
	out := make([]*billing.LineItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, billing.LineItemFromRow(billing.LineItemRowFromInvoice(rows[i])))
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
