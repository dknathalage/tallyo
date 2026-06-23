package billing

// NDIS line validation engine (spec §6) — the core compliance differentiator.
//
// This unit validates and normalises invoice/estimate line items at the SERVICE
// boundary before they reach the repository. It is shared verbatim by the
// invoice and estimate services (estimates parallel invoices).
//
// For a SUPPORT-ITEM line (a line carrying an NDIS support-item code, not a
// custom item) it enforces, in order (spec §6 steps 1-6):
//
//	1. resolve the catalog_version whose [effective_from, effective_to|∞] window
//	   contains the line's service_date;
//	2. find the support_item by code within that version, snapshotting code +
//	   name/description and pinning price_list_version_id onto the line;
//	3. look up the price_cap for the TENANT's configured zone (business_profile);
//	4. assert unit_price ≤ price_cap (skipped when the cap is NULL — a quotable
//	   item, spec §6 step 4);
//	5. assert service_date ∈ [client.plan_start, client.plan_end];
//	6. set taxable from the support item (the catalogue is authoritative).
//
// For a CUSTOM-ITEM line it skips steps 1-5 and only checks quantity ≥ 0 and
// unit_price ≥ 0. Either way the line_total is recomputed (Round2) and the
// per-document totals are derived from the validated lines.
//
// TAX-CONTRACT DECISION (2026-06-16, for J12): tax is now COMPUTED from the
// lines, not trusted from the client. NDIS supports are largely GST-free, so a
// non-taxable line contributes 0 tax; every taxable line contributes
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

	"github.com/dknathalage/tallyo/internal/businessprofile"
	"github.com/dknathalage/tallyo/internal/client"
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

// LineValidator runs the NDIS line validation engine. It depends only on the
// global catalogue, the tenant business profile (for the zone), the tenant's
// clients (for the plan window) and the tenant's tax rates (for the
// computed tax). It holds no mutable state beyond those repositories.
type LineValidator struct {
	cat      *pricelist.ItemsRepo
	profiles *businessprofile.BusinessProfileRepo
	clients  *client.ClientsRepo
	taxRates *taxrate.TaxRatesRepo
}

// NewLineValidator constructs the engine. The catalogue is read from the
// CONTROL DB (shared reference data); the business profile, clients and tax
// rates are read from the TENANT DB. In single-DB mode (tests) pass the same
// handle for both. Nil handles are a programmer error.
func NewLineValidator(tenant, control db.Executor) *LineValidator {
	if tenant == nil || control == nil {
		panic("NewLineValidator: nil db")
	}
	return &LineValidator{
		cat:      pricelist.NewItems(control),
		profiles: businessprofile.NewBusinessProfile(tenant),
		clients:  client.NewClients(tenant),
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
func (v *LineValidator) Validate(ctx context.Context, tenantID, clientID int64, items []LineItemInput) (*ValidationResult, error) {
	return v.validate(ctx, tenantID, clientID, items, false)
}

// ValidateFilling runs the engine in catalogue-authoritative pricing mode: for a
// support-item line it OVERWRITES unit_price with the resolved zone price cap
// (a quotable item with no cap keeps the caller's price, or errors when that is
// ≤ 0). Used by the agent's create path so the model only chooses code, service
// date and quantity — the platform owns the price. The human UI path uses
// Validate (caller-supplied price, capped) so providers may bill sub-cap.
func (v *LineValidator) ValidateFilling(ctx context.Context, tenantID, clientID int64, items []LineItemInput) (*ValidationResult, error) {
	return v.validate(ctx, tenantID, clientID, items, true)
}

func (v *LineValidator) validate(ctx context.Context, tenantID, clientID int64, items []LineItemInput, fillPrice bool) (*ValidationResult, error) {
	if tenantID == 0 {
		return nil, fmt.Errorf("validate lines: tenant id required")
	}
	if clientID == 0 {
		return nil, fmt.Errorf("validate lines: client id required")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("validate lines: at least one line item is required")
	}

	zone, err := v.tenantZone(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	planStart, planEnd, err := v.planWindow(ctx, tenantID, clientID)
	if err != nil {
		return nil, err
	}
	taxRate, err := v.defaultTaxRate(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	out := make([]LineItemInput, len(items))
	copy(out, items)
	var ve ValidationError
	for i := range out { // bounded by len(out)
		v.validateLine(ctx, i, zone, planStart, planEnd, &out[i], &ve, fillPrice)
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
func (v *LineValidator) validateLine(ctx context.Context, idx int, zone, planStart, planEnd string, line *LineItemInput, ve *ValidationError, fillPrice bool) {
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
		// Custom-item line: skip the catalogue checks (spec §6) entirely. The
		// repository recomputes line_total = Round2(qty*unitPrice) on write.
		return
	}

	v.validateSupportLine(ctx, idx, zone, planStart, planEnd, line, ve, fillPrice)
}

// validateSupportLine runs steps 1-6 for a support-item line, mutating the line
// (snapshots, pinned version, set taxable) and appending failures to ve.
func (v *LineValidator) validateSupportLine(ctx context.Context, idx int, zone, planStart, planEnd string, line *LineItemInput, ve *ValidationError, fillPrice bool) {
	if line.ServiceDate == "" {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "serviceDate", Message: "service date is required for an NDIS support item"})
		return
	}

	// Step 1: resolve the catalogue version. A line that already carries a pinned
	// CatalogVersionID is an EXISTING (edited) line — honour its pinned version so
	// re-validating an already-priced invoice/estimate never re-prices it against a
	// newer catalogue version (prices are frozen at create time). Only a NEW line
	// (no pinned version) resolves by service date and gets pinned.
	versionID, versionUUID, versionLabel, ok := v.resolveVersion(ctx, idx, line, ve)
	if !ok {
		return
	}

	// Step 2: find the item by code within that version; snapshot.
	item, err := v.cat.GetItemByCode(ctx, versionID, line.Code)
	if err != nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "code", Message: "could not look up that support item code"})
		return
	}
	if item == nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "code", Message: fmt.Sprintf("support item code %q is not in the %s price catalogue", line.Code, versionLabel)})
		return
	}
	snapshotSupportItem(line, versionUUID, item)

	// Step 3 + 4: resolve the zone price, then either assert unit_price ≤ cap
	// (default) or, in fill mode, OVERWRITE unit_price with the cap. For a GENERIC
	// (non-NDIS) tenant — no configured zone — the NDIS zone-cap path is SKIPPED;
	// instead, a coded line whose item carries a generic unit_price is filled from
	// it when the caller supplied no positive price (mirrors the NDIS fill but uses
	// items.unit_price, not a zone cap). A positive caller price is always kept; an
	// item with no unit_price keeps the caller's price (free-form).
	if zone != "" {
		v.applyZonePrice(ctx, idx, versionID, zone, line, ve, fillPrice)
	} else {
		applyItemUnitPrice(line, item)
	}

	// Step 5: service date within the client plan window.
	assertPlanWindow(idx, planStart, planEnd, line.ServiceDate, ve)
}

// resolveVersion picks the catalogue version a support line validates against.
// A pinned line (CatalogVersionID set — an existing/edited line) resolves to that
// exact version so its price cap never shifts under a newer catalogue; a fresh
// line resolves by service date. Returns (versionID, label, ok); on failure it
// has already appended the field error.
// Returns (versionID for downstream control-DB lookups, versionUUID to pin onto
// the tenant line, label, ok).
func (v *LineValidator) resolveVersion(ctx context.Context, idx int, line *LineItemInput, ve *ValidationError) (int64, string, string, bool) {
	if line.PriceListVersionID != nil && *line.PriceListVersionID != "" {
		ver, err := v.cat.GetVersionByUUID(ctx, *line.PriceListVersionID)
		if err != nil || ver == nil {
			ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "code", Message: "the price-catalogue version pinned to this line could not be found"})
			return 0, "", "", false
		}
		return ver.ID, ver.UUID, ver.Label, true
	}
	ver, err := v.cat.ResolveVersionForDate(ctx, line.ServiceDate)
	if err != nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "serviceDate", Message: "could not resolve a price catalogue for that service date"})
		return 0, "", "", false
	}
	if ver == nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "serviceDate", Message: fmt.Sprintf("no NDIS price catalogue is in effect for service date %s", line.ServiceDate)})
		return 0, "", "", false
	}
	return ver.ID, ver.UUID, ver.Label, true
}

// applyZonePrice looks up the tenant-zone price for the line's code and, by
// default, asserts unit_price ≤ price_cap (spec §6 step 4). When fillPrice is
// true it instead OVERWRITES unit_price with the cap (catalogue-authoritative
// pricing for the agent path). A nil cap (quotable item) has no fixed price:
// the cap assertion is skipped, and in fill mode the caller-supplied price is
// kept — but a quotable line with unit_price ≤ 0 is a failure (no price to
// apply). A missing price row is itself a failure.
func (v *LineValidator) applyZonePrice(ctx context.Context, idx int, versionID int64, zone string, line *LineItemInput, ve *ValidationError, fillPrice bool) {
	price, err := v.cat.ResolveZonePrice(ctx, versionID, line.Code, zone)
	if err != nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "unitPrice", Message: "could not look up the price cap for your zone"})
		return
	}
	if price == nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "unitPrice", Message: fmt.Sprintf("no price is published for code %q in zone %q", line.Code, zone)})
		return
	}
	if price.PriceCap == nil {
		// Quotable item: no fixed cap. In fill mode there is no price to apply,
		// so the caller must supply a positive one.
		if fillPrice && Round2(line.UnitPrice) <= 0 {
			ve.Errors = append(ve.Errors, FieldError{
				Line:    idx,
				Field:   "unitPrice",
				Message: fmt.Sprintf("code %q is a quotable item with no published price — supply an explicit unit price", line.Code),
			})
		}
		return
	}
	priceCap := *price.PriceCap
	if fillPrice {
		// Catalogue-authoritative: the platform owns the price. The model's
		// unit_price (if any) is ignored — it cannot misprice a coded line.
		line.UnitPrice = Round2(priceCap)
		return
	}
	// Compare at cent granularity (Round2 both sides) so float representation
	// noise can't spuriously fail an at-cap price; the cap is a money value.
	if Round2(line.UnitPrice) > Round2(priceCap) {
		ve.Errors = append(ve.Errors, FieldError{
			Line:    idx,
			Field:   "unitPrice",
			Message: fmt.Sprintf("unit price %.2f exceeds the NDIS price cap of %.2f for zone %q", line.UnitPrice, priceCap, zone),
		})
	}
}

// applyItemUnitPrice fills a generic coded line's unit price from the catalogue
// item's generic unit_price (the non-NDIS pricing path). It only fills when the
// item carries a positive unit_price AND the caller supplied none (UnitPrice ≤ 0);
// a caller-supplied positive price is kept, and an item with no unit_price leaves
// the line untouched (free-form). No cap is enforced — there is no NDIS zone.
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

// assertPlanWindow asserts serviceDate ∈ [planStart, planEnd] (spec §6 step 5).
// Empty bounds are treated as open (the client has no recorded plan dates),
// which is permissive by design — plan-date capture is a client concern.
func assertPlanWindow(idx int, planStart, planEnd, serviceDate string, ve *ValidationError) {
	if planStart != "" && serviceDate < planStart {
		ve.Errors = append(ve.Errors, FieldError{
			Line:    idx,
			Field:   "serviceDate",
			Message: fmt.Sprintf("service date %s is before the client's plan start %s", serviceDate, planStart),
		})
	}
	if planEnd != "" && serviceDate > planEnd {
		ve.Errors = append(ve.Errors, FieldError{
			Line:    idx,
			Field:   "serviceDate",
			Message: fmt.Sprintf("service date %s is after the client's plan end %s", serviceDate, planEnd),
		})
	}
}

// snapshotSupportItem pins the resolved version and snapshots the support item's
// identity onto the line (spec §6 step 2 + step 6). The description is filled
// from the item name only when the caller left it blank.
//
// taxable is set UNCONDITIONALLY from the catalogue item: the NDIS catalogue is
// authoritative for a support item's tax status (spec §6 step 6). A client must
// not be able to flip a taxable item to non-taxable (or vice versa) by sending
// its own taxable flag, which would corrupt the computed tax. Custom-item lines
// keep their client-controlled taxable (they never reach this function).
func snapshotSupportItem(line *LineItemInput, versionUUID string, item *pricelist.Item) {
	id := item.UUID
	line.ItemID = &id
	vid := versionUUID
	line.PriceListVersionID = &vid
	line.Code = item.Code
	if line.Description == "" {
		line.Description = item.Name
	}
	line.Taxable = item.Taxable
}

// isSupportItemLine reports whether a line is an NDIS support-item line (it
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

// tenantZone reads the tenant's configured NDIS zone. It returns "" when no
// business profile exists or its zone is unset — a GENERIC (non-NDIS) tenant.
// An empty zone tells the caller to SKIP the zone-price / cap-assert block
// entirely (data-presence gating, Phase 6); a configured zone runs the full
// NDIS price-cap path exactly as before.
func (v *LineValidator) tenantZone(ctx context.Context, tenantID int64) (string, error) {
	bp, err := v.profiles.Get(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("validate lines: read business zone: %w", err)
	}
	if bp == nil || bp.Zone == "" {
		return "", nil
	}
	return bp.Zone, nil
}

// planWindow reads the client's plan window. A missing client is a
// caller error (the repository would reject the write anyway).
func (v *LineValidator) planWindow(ctx context.Context, tenantID, clientID int64) (start, end string, err error) {
	p, err := v.clients.GetByID(ctx, tenantID, clientID)
	if err != nil {
		return "", "", fmt.Errorf("validate lines: read client: %w", err)
	}
	if p == nil {
		return "", "", fmt.Errorf("validate lines: client %d not found", clientID)
	}
	return p.PlanStart, p.PlanEnd, nil
}

// defaultTaxRate reads the tenant's default tax rate (0 when none is set).
func (v *LineValidator) defaultTaxRate(ctx context.Context, tenantID int64) (float64, error) {
	tr, err := v.taxRates.GetDefault(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("validate lines: read default tax rate: %w", err)
	}
	if tr == nil {
		return 0, nil
	}
	return tr.Rate, nil
}
