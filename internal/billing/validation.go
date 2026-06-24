package billing

// Line validation engine (spec §6) — the core compliance differentiator.
//
// This unit validates and normalises invoice/estimate line items at the SERVICE
// boundary before they reach the repository. It is shared verbatim by the
// invoice and estimate services (estimates parallel invoices).
//
// For a CATALOGUE line (a line carrying a price-list item code, not a
// custom item) it enforces, in order:
//
//	1. resolve the catalog_version whose [effective_from, effective_to|∞] window
//	   contains the line's service_date;
//	2. find the support_item by code within that version, snapshotting code +
//	   name/description and pinning price_list_version_id onto the line;
//	3. fill unit_price from the catalogue item's generic unit_price when the
//	   caller supplied no positive price (a free-form item with no unit_price
//	   keeps the caller's price);
//	4. set taxable from the support item (the catalogue is authoritative).
//
// For a CUSTOM-ITEM line it skips the catalogue steps and only checks
// quantity ≥ 0 and unit_price ≥ 0. Either way the line_total is recomputed
// (Round2) and the per-document totals are derived from the validated lines.
//
// TAX-CONTRACT DECISION (2026-06-16, for J12): tax is now COMPUTED from the
// lines, not trusted from the client. A non-taxable catalogue line (e.g. a
// GST-free support) contributes 0 tax; every taxable line contributes
// Round2(line_total * defaultTaxRate). The tenant default tax rate is read from
// tax_rates (is_default = 1); when no default exists, tax is 0. The result is
// handed to the repository through the existing InvoiceInput.Tax /
// EstimateInput.Tax field, so the repository write path is unchanged — only the
// SOURCE of the tax value moved from the client to the engine. The frontend
// (J12) should therefore treat tax as read-only/derived.
//
// Money stays REAL; every total boundary is rounded to the cent (Round2) to
// bound cumulative float drift (spec §6 money note).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/pricelist"
	"github.com/dknathalage/tallyo/internal/taxrate"
)

// FieldError is one structured, field-level validation failure. Line is the
// zero-based index of the offending line item; Field names the offending field
// (e.g. "code", "unitPrice", "serviceDate"); Message is a human-readable reason
// the HTTP layer (and J12) can surface inline.
type FieldError struct {
	Line    int    `json:"line"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError aggregates one or more FieldErrors. Returning it (rather than
// a bare error) lets the HTTP layer respond 422 with per-line, per-field detail.
type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

// Error renders the aggregated failures as a single string (the error
// interface). The structured slice in Errors is what callers should surface.
func (e *ValidationError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "validation failed"
	}
	parts := make([]string, 0, len(e.Errors))
	for i := range e.Errors { // bounded by len(e.Errors)
		fe := e.Errors[i]
		parts = append(parts, fmt.Sprintf("line %d: %s: %s", fe.Line, fe.Field, fe.Message))
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// AsValidationError reports whether err is (or wraps) a *ValidationError, and
// returns it. Used by the HTTP layer to choose a 422 response with details.
func AsValidationError(err error) (*ValidationError, bool) {
	if err == nil {
		return nil, false
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}

// LineValidator runs the line validation engine. It depends only on the
// tenant price catalogue and the tenant's tax rates (for the computed tax).
// It holds no mutable state beyond those repositories.
type LineValidator struct {
	cat      *pricelist.ItemsRepo
	taxRates *taxrate.TaxRatesRepo
}

// NewLineValidator constructs the engine. ALL reads — catalogue/price list and
// tax rates — come from the TENANT DB (the price list is tenant-owned). A nil
// handle is a programmer error.
func NewLineValidator(tenant db.Executor) *LineValidator {
	if tenant == nil {
		panic("NewLineValidator: nil db")
	}
	return &LineValidator{
		cat:      pricelist.NewItems(tenant),
		taxRates: taxrate.NewTaxRates(tenant),
	}
}

// ValidationResult carries the normalised line items (snapshots pinned, taxable
// set, line_total recomputed) plus the engine-computed tax. The caller
// passes Items and Tax straight to the repository write path.
type ValidationResult struct {
	Items []LineItemInput
	Tax   float64
}

// Validate runs the full engine for one document's lines against a client.
// It returns the normalised lines + computed tax, or a *ValidationError listing
// every field-level failure (it validates all lines, not just the first).
//
// Invariants (NASA rule 5): tenantID and clientID must be non-zero and at
// least one line must be present; violations are programmer errors surfaced as
// plain errors (the caller's repository would reject them anyway).
func (v *LineValidator) Validate(ctx context.Context, tenantID, clientID string, items []LineItemInput) (*ValidationResult, error) {
	return v.validate(ctx, tenantID, clientID, items)
}

// ValidateFilling is kept as a thin alias for Validate: both now price a
// catalogue line from the item's generic unit_price (filling when the caller
// supplied no positive price). It is retained so the agent create path and the
// session pricing path don't need to change call sites.
func (v *LineValidator) ValidateFilling(ctx context.Context, tenantID, clientID string, items []LineItemInput) (*ValidationResult, error) {
	return v.validate(ctx, tenantID, clientID, items)
}

func (v *LineValidator) validate(ctx context.Context, tenantID, clientID string, items []LineItemInput) (*ValidationResult, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("validate lines: tenant id required")
	}
	if clientID == "" {
		return nil, fmt.Errorf("validate lines: client id required")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("validate lines: at least one line item is required")
	}

	taxRate, err := v.defaultTaxRate(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	out := make([]LineItemInput, len(items))
	copy(out, items)
	var ve ValidationError
	for i := range out { // bounded by len(out)
		v.validateLine(ctx, tenantID, i, &out[i], &ve)
	}
	if len(ve.Errors) > 0 {
		return nil, &ve
	}

	tax := computeLineTax(out, taxRate)
	return &ValidationResult{Items: out, Tax: tax}, nil
}

// validateLine validates and normalises a single line in place, appending any
// failures to ve. Support-item lines run the full catalogue flow; custom-item
// lines run only the non-negativity checks. Errors are accumulated, not thrown,
// so the caller collects every problem in one pass.
func (v *LineValidator) validateLine(ctx context.Context, tenantID string, idx int, line *LineItemInput, ve *ValidationError) {
	if line == nil {
		return
	}
	if line.Quantity < 0 {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "quantity", Message: "quantity must not be negative"})
	}
	if line.UnitPrice < 0 {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "unitPrice", Message: "unit price must not be negative"})
	}

	if !isSupportItemLine(line) {
		// Custom-item line: skip the catalogue checks entirely. The repository
		// recomputes line_total = Round2(qty*unitPrice) on write.
		return
	}

	v.validateSupportLine(ctx, tenantID, idx, line, ve)
}

// validateSupportLine runs the catalogue steps for a support-item line, mutating
// the line (snapshots, pinned version, filled price, set taxable) and appending
// failures to ve.
func (v *LineValidator) validateSupportLine(ctx context.Context, tenantID string, idx int, line *LineItemInput, ve *ValidationError) {
	if line.ServiceDate == "" {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "serviceDate", Message: "service date is required for a catalogue item"})
		return
	}

	// Step 1: resolve the catalogue version. A line that already carries a pinned
	// CatalogVersionID is an EXISTING (edited) line — honour its pinned version so
	// re-validating an already-priced invoice/estimate never re-prices it against a
	// newer catalogue version (prices are frozen at create time). Only a NEW line
	// (no pinned version) resolves by service date and gets pinned.
	versionID, versionUUID, versionLabel, ok := v.resolveVersion(ctx, tenantID, idx, line, ve)
	if !ok {
		return
	}

	// Step 2: find the item by code within that version; snapshot.
	item, err := v.cat.GetItemByCode(ctx, tenantID, versionID, line.Code)
	if err != nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "code", Message: "could not look up that support item code"})
		return
	}
	if item == nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "code", Message: fmt.Sprintf("support item code %q is not in the %s price catalogue", line.Code, versionLabel)})
		return
	}
	snapshotSupportItem(line, versionUUID, item)

	// Step 3: fill the unit price from the catalogue item's generic unit_price
	// when the caller supplied no positive price. A positive caller price is
	// always kept; an item with no unit_price keeps the caller's price (free-form).
	applyItemUnitPrice(line, item)
}

// resolveVersion picks the catalogue version a support line validates against.
// A pinned line (CatalogVersionID set — an existing/edited line) resolves to that
// exact version so its price never shifts under a newer catalogue; a fresh
// line resolves by service date. Returns (versionID, label, ok); on failure it
// has already appended the field error.
// Returns (versionID for downstream control-DB lookups, versionUUID to pin onto
// the tenant line, label, ok).
func (v *LineValidator) resolveVersion(ctx context.Context, tenantID string, idx int, line *LineItemInput, ve *ValidationError) (string, string, string, bool) {
	if line.PriceListVersionID != nil && *line.PriceListVersionID != "" {
		ver, err := v.cat.GetVersionByUUID(ctx, tenantID, *line.PriceListVersionID)
		if err != nil || ver == nil {
			ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "code", Message: "the price-catalogue version pinned to this line could not be found"})
			return "", "", "", false
		}
		return ver.ID, ver.ID, ver.Label, true
	}
	ver, err := v.cat.ResolveVersionForDate(ctx, tenantID, line.ServiceDate)
	if err != nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "serviceDate", Message: "could not resolve a price catalogue for that service date"})
		return "", "", "", false
	}
	if ver == nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "serviceDate", Message: fmt.Sprintf("no price list is in effect for service date %s", line.ServiceDate)})
		return "", "", "", false
	}
	return ver.ID, ver.ID, ver.Label, true
}

// applyItemUnitPrice fills a coded line's unit price from the catalogue item's
// generic unit_price. It only fills when the item carries a positive unit_price
// AND the caller supplied none (UnitPrice ≤ 0); a caller-supplied positive price
// is kept, and an item with no unit_price leaves the line untouched (free-form).
func applyItemUnitPrice(line *LineItemInput, item *pricelist.Item) {
	if line == nil || item == nil {
		return
	}
	if item.UnitPrice == nil {
		return
	}
	if Round2(line.UnitPrice) > 0 {
		return
	}
	line.UnitPrice = Round2(*item.UnitPrice)
}

// snapshotSupportItem pins the resolved version and snapshots the support item's
// identity onto the line (spec §6 step 2 + step 6). The description is filled
// from the item name only when the caller left it blank.
//
// taxable is set UNCONDITIONALLY from the catalogue item: the price catalogue is
// authoritative for a catalogue item's tax status (spec §6 step 6). A client must
// not be able to flip a taxable item to non-taxable (or vice versa) by sending
// its own taxable flag, which would corrupt the computed tax. Custom-item lines
// keep their client-controlled taxable (they never reach this function).
func snapshotSupportItem(line *LineItemInput, versionUUID string, item *pricelist.Item) {
	id := item.ID
	line.ItemID = &id
	vid := versionUUID
	line.PriceListVersionID = &vid
	line.Code = item.Code
	if line.Description == "" {
		line.Description = item.Name
	}
	line.Taxable = item.Taxable
}

// isSupportItemLine reports whether a line is a catalogue line (it
// carries a code and is not a custom item). Custom-item lines carry a
// CustomItemID and no catalogue code.
func isSupportItemLine(line *LineItemInput) bool {
	if line.CustomItemID != nil {
		return false
	}
	return strings.TrimSpace(line.Code) != ""
}

// computeLineTax sums Round2(line_total * rate) over the taxable lines,
// where line_total = Round2(qty*unitPrice) — matching the repository's own
// rounding so the engine's tax agrees with the persisted subtotal. Non-taxable
// lines contribute zero. The total is rounded to the cent (spec §6 money note).
func computeLineTax(items []LineItemInput, rate float64) float64 {
	if rate <= 0 {
		return 0
	}
	var tax float64
	for i := range items { // bounded by len(items)
		if !items[i].Taxable {
			continue
		}
		lineTotal := Round2(items[i].Quantity * items[i].UnitPrice)
		tax += Round2(lineTotal * rate)
	}
	return Round2(tax)
}

// defaultTaxRate reads the tenant's default tax rate (0 when none is set).
func (v *LineValidator) defaultTaxRate(ctx context.Context, tenantID string) (float64, error) {
	tr, err := v.taxRates.GetDefault(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("validate lines: read default tax rate: %w", err)
	}
	if tr == nil {
		return 0, nil
	}
	return tr.Rate, nil
}
