// Package billing owns shared line-item types used across invoices, estimates,
// and recurring templates.
package billing

// LineItem is the domain view of a row in the line_items table.
type LineItem struct {
	ID               int64   `json:"id"`
	UUID             string  `json:"uuid"`
	SupportItemID    *int64  `json:"supportItemId"`
	CustomItemID     *int64  `json:"customItemId"`
	CatalogVersionID *int64  `json:"catalogVersionId"`
	Code             string  `json:"code"`
	Description      string  `json:"description"`
	ServiceDate      string  `json:"serviceDate"`
	Unit             string  `json:"unit"`
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
	Quantity         float64 `json:"quantity"`
	UnitPrice        float64 `json:"unitPrice"`
	GstFree          bool    `json:"gstFree"`
	SortOrder        int64   `json:"sortOrder"`
}
