// Package billing owns shared line-item types used across invoices, estimates,
// and recurring templates.
package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db/gen"
)

// ResolveCatalogueItemID validates an inbound catalogue-item uuid against the
// tenant and returns it for storage on a line. An empty/nil uuid -> NULL FK; an
// unknown uuid -> ErrUnknownCatalogueItem so the handler can 400. The stored
// value IS the catalogue version-row uuid (line items pin the exact version).
func ResolveCatalogueItemID(ctx context.Context, q *gen.Queries, tenantID string, catalogueItemUUID *string) (sql.NullString, error) {
	if catalogueItemUUID == nil || *catalogueItemUUID == "" {
		return sql.NullString{}, nil
	}
	_, err := q.GetCatalogueItem(ctx, gen.GetCatalogueItemParams{TenantID: tenantID, ID: *catalogueItemUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, fmt.Errorf("%w: %q", ErrUnknownCatalogueItem, *catalogueItemUUID)
	}
	if err != nil {
		return sql.NullString{}, fmt.Errorf("resolve catalogue item uuid: %w", err)
	}
	return sql.NullString{String: *catalogueItemUUID, Valid: true}, nil
}

// ErrUnknownCatalogueItem is returned by ResolveCatalogueItemID when an inbound
// catalogue-item uuid matches no tenant catalogue item. Handlers map it to a 400.
var ErrUnknownCatalogueItem = errors.New("unknown catalogue item")

// LineItemRow is the joined central line_items row (the row + the related
// catalogue-item uuid). The four by-* reads of line_items all produce the same
// shape; each gen row type is converted to this before mapping so the API can
// surface catalogueItemId as the catalogue-item uuid.
type LineItemRow struct {
	ID                string
	SessionID         sql.NullString
	InvoiceID         sql.NullString
	CatalogueItemID   sql.NullString
	CatalogueItemUuid sql.NullString
	Code              sql.NullString
	Description       string
	ServiceDate       sql.NullString
	Unit              sql.NullString
	StartTime         sql.NullString
	EndTime           sql.NullString
	Quantity          float64
	UnitPrice         float64
	Taxable           int64
	LineTotal         float64
	SortOrder         sql.NullInt64
}

// LineItemFromRow maps one joined central line_items row to the domain shape.
// Shared by the invoice and session slices (both read the central line_items
// table). catalogueItemId surfaces as the catalogue-item uuid (nil when no
// catalogue link); the row id stays internal.
func LineItemFromRow(row LineItemRow) *LineItem {
	return &LineItem{
		ID:              row.ID,
		SessionID:       ptrStr(row.SessionID),
		InvoiceID:       ptrStr(row.InvoiceID),
		CatalogueItemID: ptrStr(row.CatalogueItemUuid),
		Code:            row.Code.String,
		Description:     row.Description,
		ServiceDate:     row.ServiceDate.String,
		Unit:            row.Unit.String,
		StartTime:       row.StartTime.String,
		EndTime:         row.EndTime.String,
		Quantity:        row.Quantity,
		UnitPrice:       row.UnitPrice,
		Taxable:         row.Taxable == 1,
		LineTotal:       row.LineTotal,
		SortOrder:       row.SortOrder.Int64,
	}
}

// LineItemRowFromInvoice/Session/Get/SessionUUID adapt the four generated joined
// row types (all structurally identical) into the shared LineItemRow.
func LineItemRowFromInvoice(r gen.ListLineItemsForInvoiceRow) LineItemRow {
	return LineItemRow{
		ID: r.ID, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		CatalogueItemID: r.CatalogueItemID, CatalogueItemUuid: r.CatalogueItemUuid,
		Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

func LineItemRowFromSessionList(r gen.ListLineItemsForSessionRow) LineItemRow {
	return LineItemRow{
		ID: r.ID, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		CatalogueItemID: r.CatalogueItemID, CatalogueItemUuid: r.CatalogueItemUuid,
		Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

func LineItemRowFromGet(r gen.GetLineItemRow) LineItemRow {
	return LineItemRow{
		ID: r.ID, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		CatalogueItemID: r.CatalogueItemID, CatalogueItemUuid: r.CatalogueItemUuid,
		Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

func LineItemRowFromSessionUUID(r gen.GetSessionLineItemByUUIDRow) LineItemRow {
	return LineItemRow{
		ID: r.ID, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		CatalogueItemID: r.CatalogueItemID, CatalogueItemUuid: r.CatalogueItemUuid,
		Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

func ptrStr(n sql.NullString) *string {
	if !n.Valid || n.String == "" {
		return nil
	}
	v := n.String
	return &v
}

// LineItem is the domain view of a row in the line_items table. A line item is
// the same row whether it lives on a session (SessionID set, InvoiceID nil) or on
// an invoice (InvoiceID set); drafting links it by setting InvoiceID.
type LineItem struct {
	ID              string  `json:"id"`              // public identifier (item uuid)
	SessionID       *string `json:"-"`               // internal parent FK (redundant on the embedded API)
	InvoiceID       *string `json:"-"`               // internal parent FK (redundant on the embedded API)
	CatalogueItemID *string `json:"catalogueItemId"` // catalogue version-row uuid (nil when no catalogue link)
	Code            string  `json:"code"`
	Description     string  `json:"description"`
	ServiceDate     string  `json:"serviceDate"`
	Unit            string  `json:"unit"`
	StartTime       string  `json:"startTime"` // time-class units only
	EndTime         string  `json:"endTime"`   // time-class units only
	Quantity        float64 `json:"quantity"`
	UnitPrice       float64 `json:"unitPrice"`
	Taxable         bool    `json:"taxable"`
	LineTotal       float64 `json:"lineTotal"`
	SortOrder       int64   `json:"sortOrder"`
}

// LineItemInput is the writable subset of a line item. LineTotal is computed
// (round2(quantity*unitPrice)) when not explicitly supplied.
type LineItemInput struct {
	CatalogueItemID *string `json:"catalogueItemId"` // catalogue version-row uuid (validated against the tenant at the write boundary)
	Code            string  `json:"code"`
	Description     string  `json:"description"`
	ServiceDate     string  `json:"serviceDate"`
	Unit            string  `json:"unit"`
	StartTime       string  `json:"startTime"` // time-class units only
	EndTime         string  `json:"endTime"`   // time-class units only
	Quantity        float64 `json:"quantity"`
	UnitPrice       float64 `json:"unitPrice"`
	Taxable         bool    `json:"taxable"`
	SortOrder       int64   `json:"sortOrder"`
}
