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

// ResolveCustomItemID translates an inbound custom-item uuid into its int FK,
// tenant-scoped, for a line-item write. An empty/nil uuid → NULL FK; an unknown
// uuid → ErrUnknownCustomItem so the handler can 400. The int FK never crosses
// the API — storage stays int-based, resolved here at the write boundary.
func ResolveCustomItemID(ctx context.Context, q *gen.Queries, tenantID string, customItemUUID *string) (sql.NullString, error) {
	if customItemUUID == nil || *customItemUUID == "" {
		return sql.NullString{}, nil
	}
	id, err := q.GetCustomItemIDByUUID(ctx, gen.GetCustomItemIDByUUIDParams{TenantID: tenantID, ID: *customItemUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, fmt.Errorf("%w: %q", ErrUnknownCustomItem, *customItemUUID)
	}
	if err != nil {
		return sql.NullString{}, fmt.Errorf("resolve custom item uuid: %w", err)
	}
	return sql.NullString{String: id, Valid: true}, nil
}

// ErrUnknownCustomItem is returned by ResolveCustomItemID when an inbound
// custom-item uuid matches no tenant custom item. Handlers map it to a 400.
var ErrUnknownCustomItem = errors.New("unknown custom item")

// LineItemRow is the joined central line_items row (the row + the related
// custom-item uuid). The four by-* reads of line_items all produce the same
// shape; each gen row type is converted to this before mapping so the API can
// surface customItemId as the custom-item uuid rather than the int FK.
type LineItemRow struct {
	ID                 string
	SessionID          sql.NullString
	InvoiceID          sql.NullString
	ItemID             sql.NullString
	CustomItemID       sql.NullString
	CustomItemUuid     sql.NullString
	PriceListVersionID sql.NullString
	Code               sql.NullString
	Description        string
	ServiceDate        sql.NullString
	Unit               sql.NullString
	StartTime          sql.NullString
	EndTime            sql.NullString
	Quantity           float64
	UnitPrice          float64
	Taxable            int64
	LineTotal          float64
	SortOrder          sql.NullInt64
}

// LineItemFromRow maps one joined central line_items row to the domain shape.
// Shared by the invoice and session slices (both read the central line_items
// table). customItemId surfaces as the custom-item uuid (nil when no custom
// item); the int FK stays internal.
func LineItemFromRow(row LineItemRow) *LineItem {
	return &LineItem{
		ID:                 row.ID,
		SessionID:          ptrStr(row.SessionID),
		InvoiceID:          ptrStr(row.InvoiceID),
		ItemID:             ptrStr(row.ItemID),
		CustomItemID:       ptrStr(row.CustomItemID),
		CustomItemUUID:     ptrStr(row.CustomItemUuid),
		PriceListVersionID: ptrStr(row.PriceListVersionID),
		Code:               row.Code.String,
		Description:        row.Description,
		ServiceDate:        row.ServiceDate.String,
		Unit:               row.Unit.String,
		StartTime:          row.StartTime.String,
		EndTime:            row.EndTime.String,
		Quantity:           row.Quantity,
		UnitPrice:          row.UnitPrice,
		Taxable:            row.Taxable == 1,
		LineTotal:          row.LineTotal,
		SortOrder:          row.SortOrder.Int64,
	}
}

// LineItemRowFromInvoice/Session/Get/SessionUUID adapt the four generated joined
// row types (all structurally identical) into the shared LineItemRow.
func LineItemRowFromInvoice(r gen.ListLineItemsForInvoiceRow) LineItemRow {
	return LineItemRow{
		ID: r.ID, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		ItemID: r.ItemID, CustomItemID: r.CustomItemID, CustomItemUuid: r.CustomItemUuid,
		PriceListVersionID: r.PriceListVersionID, Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

func LineItemRowFromSessionList(r gen.ListLineItemsForSessionRow) LineItemRow {
	return LineItemRow{
		ID: r.ID, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		ItemID: r.ItemID, CustomItemID: r.CustomItemID, CustomItemUuid: r.CustomItemUuid,
		PriceListVersionID: r.PriceListVersionID, Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

func LineItemRowFromGet(r gen.GetLineItemRow) LineItemRow {
	return LineItemRow{
		ID: r.ID, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		ItemID: r.ItemID, CustomItemID: r.CustomItemID, CustomItemUuid: r.CustomItemUuid,
		PriceListVersionID: r.PriceListVersionID, Code: r.Code, Description: r.Description,
		ServiceDate: r.ServiceDate, Unit: r.Unit, StartTime: r.StartTime, EndTime: r.EndTime,
		Quantity: r.Quantity, UnitPrice: r.UnitPrice, Taxable: r.Taxable, LineTotal: r.LineTotal, SortOrder: r.SortOrder,
	}
}

func LineItemRowFromSessionUUID(r gen.GetSessionLineItemByUUIDRow) LineItemRow {
	return LineItemRow{
		ID: r.ID, SessionID: r.SessionID, InvoiceID: r.InvoiceID,
		ItemID: r.ItemID, CustomItemID: r.CustomItemID, CustomItemUuid: r.CustomItemUuid,
		PriceListVersionID: r.PriceListVersionID, Code: r.Code, Description: r.Description,
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
// the same row whether it lives on a session (SessionID set, InvoiceID nil) or on an
// invoice (InvoiceID set); drafting links it by setting InvoiceID.
type LineItem struct {
	ID                 string  `json:"id"`                 // public identifier (item uuid)
	SessionID          *string `json:"-"`                  // internal parent FK; a line item is always fetched embedded in its parent session, so the parent ref is redundant on the API
	InvoiceID          *string `json:"-"`                  // internal parent FK; a line item is always fetched embedded in its parent invoice, so the parent ref is redundant on the API
	ItemID             *string `json:"itemId"`             // tenant items.uuid
	CustomItemID       *string `json:"-"`                  // internal tenant-local FK; the public ref is the uuid
	CustomItemUUID     *string `json:"customItemId"`       // tenant custom_items.uuid (nil when no custom item)
	PriceListVersionID *string `json:"priceListVersionId"` // tenant price_list_versions.uuid
	Code               string  `json:"code"`
	Description        string  `json:"description"`
	ServiceDate        string  `json:"serviceDate"`
	Unit               string  `json:"unit"`
	StartTime          string  `json:"startTime"` // time-class units only
	EndTime            string  `json:"endTime"`   // time-class units only
	Quantity           float64 `json:"quantity"`
	UnitPrice          float64 `json:"unitPrice"`
	Taxable            bool    `json:"taxable"`
	LineTotal          float64 `json:"lineTotal"`
	SortOrder          int64   `json:"sortOrder"`
}

// LineItemInput is the writable subset of a line item. LineTotal is computed
// (round2(quantity*unitPrice)) when not explicitly supplied.
type LineItemInput struct {
	ItemID             *string `json:"itemId"`             // tenant items.uuid
	CustomItemID       *string `json:"customItemId"`       // tenant custom_items.uuid (resolved to the int FK at the write boundary)
	PriceListVersionID *string `json:"priceListVersionId"` // tenant price_list_versions.uuid
	Code               string  `json:"code"`
	Description        string  `json:"description"`
	ServiceDate        string  `json:"serviceDate"`
	Unit               string  `json:"unit"`
	StartTime          string  `json:"startTime"` // time-class units only
	EndTime            string  `json:"endTime"`   // time-class units only
	Quantity           float64 `json:"quantity"`
	UnitPrice          float64 `json:"unitPrice"`
	Taxable            bool    `json:"taxable"`
	SortOrder          int64   `json:"sortOrder"`
}
