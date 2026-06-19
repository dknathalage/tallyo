// Package billing owns shared line-item types used across invoices, estimates,
// and recurring templates.
package billing

import (
	"database/sql"

	"github.com/dknathalage/tallyo/internal/db/gen"
)

// LineItemFromRow maps one generated line_items row to the domain shape. Shared
// by the invoice and shift slices (both read the central line_items table).
func LineItemFromRow(row gen.LineItem) *LineItem {
	return &LineItem{
		ID:               row.ID,
		UUID:             row.Uuid,
		ShiftID:          ptrInt(row.ShiftID),
		InvoiceID:        ptrInt(row.InvoiceID),
		SupportItemID:    ptrInt(row.SupportItemID),
		CustomItemID:     ptrInt(row.CustomItemID),
		CatalogVersionID: ptrInt(row.CatalogVersionID),
		Code:             row.Code.String,
		Description:      row.Description,
		ServiceDate:      row.ServiceDate.String,
		Unit:             row.Unit.String,
		StartTime:        row.StartTime.String,
		EndTime:          row.EndTime.String,
		Quantity:         row.Quantity,
		UnitPrice:        row.UnitPrice,
		GstFree:          row.GstFree == 1,
		LineTotal:        row.LineTotal,
		SortOrder:        row.SortOrder.Int64,
	}
}

func ptrInt(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// LineItem is the domain view of a row in the line_items table. A line item is
// the same row whether it lives on a shift (ShiftID set, InvoiceID nil) or on an
// invoice (InvoiceID set); drafting links it by setting InvoiceID.
type LineItem struct {
	ID               int64   `json:"id"`
	UUID             string  `json:"uuid"`
	ShiftID          *int64  `json:"shiftId"`
	InvoiceID        *int64  `json:"invoiceId"`
	SupportItemID    *int64  `json:"supportItemId"`
	CustomItemID     *int64  `json:"customItemId"`
	CatalogVersionID *int64  `json:"catalogVersionId"`
	Code             string  `json:"code"`
	Description      string  `json:"description"`
	ServiceDate      string  `json:"serviceDate"`
	Unit             string  `json:"unit"`
	StartTime        string  `json:"startTime"` // time-class units only
	EndTime          string  `json:"endTime"`   // time-class units only
	Quantity         float64 `json:"quantity"`
	UnitPrice        float64 `json:"unitPrice"`
	GstFree          bool    `json:"gstFree"`
	LineTotal        float64 `json:"lineTotal"`
	SortOrder        int64   `json:"sortOrder"`
}

// LineItemInput is the writable subset of a line item. LineTotal is computed
// (round2(quantity*unitPrice)) when not explicitly supplied.
type LineItemInput struct {
	SupportItemID    *int64  `json:"supportItemId"`
	CustomItemID     *int64  `json:"customItemId"`
	CatalogVersionID *int64  `json:"catalogVersionId"`
	Code             string  `json:"code"`
	Description      string  `json:"description"`
	ServiceDate      string  `json:"serviceDate"`
	Unit             string  `json:"unit"`
	StartTime        string  `json:"startTime"` // time-class units only
	EndTime          string  `json:"endTime"`   // time-class units only
	Quantity         float64 `json:"quantity"`
	UnitPrice        float64 `json:"unitPrice"`
	GstFree          bool    `json:"gstFree"`
	SortOrder        int64   `json:"sortOrder"`
}
