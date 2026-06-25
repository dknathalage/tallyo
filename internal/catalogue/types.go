// Package catalogue is the per-tenant catalogue slice: reusable priced line
// templates with per-item copy-on-write versioning. It replaces the former
// customitem + pricelist slices. An edit forks a new version row only when the
// current row is referenced by an invoice/estimate line; otherwise it mutates in
// place. Delete tombstones the item (is_current = 0 for the logical_id).
package catalogue

import "github.com/dknathalage/tallyo/internal/apperr"

// CatalogueItem is the domain view of a catalogue_items row. id is a specific
// version row uuid (line items pin it); logicalId is the stable identity across
// an item's versions.
type CatalogueItem struct {
	ID        string  `json:"id"`
	LogicalID string  `json:"logicalId"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	Category  string  `json:"category"`
	UnitPrice float64 `json:"unitPrice"`
	Taxable   bool    `json:"taxable"`
	Metadata  string  `json:"metadata"`
	Version   int64   `json:"version"`
	IsCurrent bool    `json:"isCurrent"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// CatalogueItemInput is the writable subset of a catalogue item.
type CatalogueItemInput struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	Category  string  `json:"category"`
	UnitPrice float64 `json:"unitPrice"`
	Taxable   bool    `json:"taxable"`
	Metadata  string  `json:"metadata"`
}

// Validate enforces the cheap required-field rules before the repo runs. A
// failure is an *apperr.ValidationError so the HTTP layer responds 422.
func (in CatalogueItemInput) Validate() error {
	ve := &apperr.ValidationError{}
	if in.Name == "" {
		ve.Errors = append(ve.Errors, apperr.FieldError{Line: 0, Field: "name", Message: "required"})
	}
	if in.UnitPrice < 0 {
		ve.Errors = append(ve.Errors, apperr.FieldError{Line: 0, Field: "unitPrice", Message: "must not be negative"})
	}
	if len(ve.Errors) > 0 {
		return ve
	}
	return nil
}
