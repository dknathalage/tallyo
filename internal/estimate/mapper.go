package estimate

// Row→domain mappers: the shared flat estimateFields shape, the gen-row adapters
// (Get/GetByID here; the list adapters live in query.go), the line-item mapper,
// and the small orDefault helper. Split out of repository.go to keep that file to
// core CRUD.

import (
	"database/sql"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// estimateFields is the shared, flat shape of every estimates join row.
type estimateFields struct {
	id, number                          string
	clientID                            sql.NullString
	clientUUID                          sql.NullString
	payerID                             sql.NullString
	payerUUID                           sql.NullString
	status, issueDate, validUntil       string
	subtotal, tax, total                float64
	notes                               sql.NullString
	convertedInvoiceID                  sql.NullString
	convertedInvoiceUUID                sql.NullString
	businessSnap, clientSnap, payerSnap sql.NullString
	createdAt, updatedAt                string
	clientName                          sql.NullString
}

// toEstimateFromRow builds a domain Estimate (without line items).
func toEstimateFromRow(f estimateFields) *Estimate {
	return &Estimate{
		ID:                   f.id,
		Number:               f.number,
		ClientID:             db.PtrStr(f.clientID),
		ClientUUID:           f.clientUUID.String,
		ClientName:           f.clientName.String,
		PayerID:              db.PtrStr(f.payerID),
		PayerUUID:            db.PtrStr(f.payerUUID),
		Status:               f.status,
		IssueDate:            f.issueDate,
		ValidUntil:           f.validUntil,
		Subtotal:             f.subtotal,
		Tax:                  f.tax,
		Total:                f.total,
		Notes:                f.notes.String,
		ConvertedInvoiceID:   db.PtrStr(f.convertedInvoiceID),
		ConvertedInvoiceUUID: db.PtrStr(f.convertedInvoiceUUID),
		BusinessSnapshot:     f.businessSnap.String,
		ClientSnapshot:       f.clientSnap.String,
		PayerSnapshot:        f.payerSnap.String,
		CreatedAt:            f.createdAt,
		UpdatedAt:            f.updatedAt,
		LineItems:            []*billing.LineItem{},
	}
}

func estimateFieldsFromGet(r gen.GetEstimateRow) estimateFields {
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

func estimateFieldsFromGetByID(r gen.GetEstimateByIDRow) estimateFields {
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

// mapEstimateLineItems maps generated joined rows to domain line items
// (non-nil); customItemId surfaces as the custom-item uuid.
func mapEstimateLineItems(rows []gen.ListEstimateLineItemsRow) []*billing.LineItem {
	out := make([]*billing.LineItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toEstimateLineItem(rows[i]))
	}
	return out
}

// toEstimateLineItem maps one generated joined estimate line item to the shared
// LineItem domain shape.
func toEstimateLineItem(row gen.ListEstimateLineItemsRow) *billing.LineItem {
	return &billing.LineItem{
		ID:                 row.ID,
		ItemID:             db.PtrStr(row.ItemID),
		CustomItemID:       db.PtrStr(row.CustomItemID),
		CustomItemUUID:     db.PtrStr(row.CustomItemUuid),
		PriceListVersionID: db.PtrStr(row.PriceListVersionID),
		Code:               row.Code.String,
		Description:        row.Description,
		ServiceDate:        row.ServiceDate.String,
		Unit:               row.Unit.String,
		Quantity:           row.Quantity,
		UnitPrice:          row.UnitPrice,
		Taxable:            row.Taxable == 1,
		LineTotal:          row.LineTotal,
		SortOrder:          row.SortOrder.Int64,
	}
}

// orDefault returns s when non-empty, otherwise def.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
