package service

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
//	   name/description and pinning catalog_version_id onto the line;
//	3. look up the price_cap for the TENANT's configured zone (business_profile);
//	4. assert unit_price ≤ price_cap (skipped when the cap is NULL — a quotable
//	   item, spec §6 step 4);
//	5. assert service_date ∈ [participant.plan_start, participant.plan_end];
//	6. default gst_free from the support item when not explicitly set.
//
// For a CUSTOM-ITEM line it skips steps 1-5 and only checks quantity ≥ 0 and
// unit_price ≥ 0. Either way the line_total is recomputed (round2) and the
// per-document totals are derived from the validated lines.
//
// TAX-CONTRACT DECISION (2026-06-16, for J12): tax is now COMPUTED from the
// lines, not trusted from the client. NDIS supports are largely GST-free, so a
// gst_free line contributes 0 tax; every other line contributes
// round2(line_total * defaultTaxRate). The tenant default tax rate is read from
// tax_rates (is_default = 1); when no default exists, tax is 0. The result is
// handed to the repository through the existing InvoiceInput.Tax /
// EstimateInput.Tax field, so the repository write path is unchanged — only the
// SOURCE of the tax value moved from the client to the engine. The frontend
// (J12) should therefore treat tax as read-only/derived.
//
// Money stays REAL; every total boundary is rounded to the cent (round2) to
// bound cumulative float drift (spec §6 money note).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/dknathalage/tallyo/internal/repository"
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
// participants (for the plan window) and the tenant's tax rates (for the
// computed tax). It holds no mutable state beyond those repositories.
type LineValidator struct {
	catalog      *repository.CatalogRepo
	profiles     *repository.BusinessProfileRepo
	participants *repository.ParticipantsRepo
	taxRates     *repository.TaxRatesRepo
}

// NewLineValidator constructs the engine. A nil db is a programmer error.
func NewLineValidator(db *sql.DB) *LineValidator {
	if db == nil {
		panic("NewLineValidator: nil db")
	}
	return &LineValidator{
		catalog:      repository.NewCatalog(db),
		profiles:     repository.NewBusinessProfile(db),
		participants: repository.NewParticipants(db),
		taxRates:     repository.NewTaxRates(db),
	}
}

// ValidationResult carries the normalised line items (snapshots pinned, gst_free
// defaulted, line_total recomputed) plus the engine-computed tax. The caller
// passes Items and Tax straight to the repository write path.
type ValidationResult struct {
	Items []repository.LineItemInput
	Tax   float64
}

// Validate runs the full engine for one document's lines against a participant.
// It returns the normalised lines + computed tax, or a *ValidationError listing
// every field-level failure (it validates all lines, not just the first).
//
// Invariants (NASA rule 5): tenantID and participantID must be non-zero and at
// least one line must be present; violations are programmer errors surfaced as
// plain errors (the caller's repository would reject them anyway).
func (v *LineValidator) Validate(ctx context.Context, tenantID, participantID int64, items []repository.LineItemInput) (*ValidationResult, error) {
	if tenantID == 0 {
		return nil, fmt.Errorf("validate lines: tenant id required")
	}
	if participantID == 0 {
		return nil, fmt.Errorf("validate lines: participant id required")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("validate lines: at least one line item is required")
	}

	zone, err := v.tenantZone(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	planStart, planEnd, err := v.planWindow(ctx, tenantID, participantID)
	if err != nil {
		return nil, err
	}
	taxRate, err := v.defaultTaxRate(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	out := make([]repository.LineItemInput, len(items))
	copy(out, items)
	var ve ValidationError
	for i := range out { // bounded by len(out)
		v.validateLine(ctx, i, zone, planStart, planEnd, &out[i], &ve)
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
func (v *LineValidator) validateLine(ctx context.Context, idx int, zone, planStart, planEnd string, line *repository.LineItemInput, ve *ValidationError) {
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
		// repository recomputes line_total = round2(qty*unitPrice) on write.
		return
	}

	v.validateSupportLine(ctx, idx, zone, planStart, planEnd, line, ve)
}

// validateSupportLine runs steps 1-6 for a support-item line, mutating the line
// (snapshots, pinned version, defaulted gst_free) and appending failures to ve.
func (v *LineValidator) validateSupportLine(ctx context.Context, idx int, zone, planStart, planEnd string, line *repository.LineItemInput, ve *ValidationError) {
	if line.ServiceDate == "" {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "serviceDate", Message: "service date is required for an NDIS support item"})
		return
	}

	// Step 1: resolve the catalogue version for the service date.
	ver, err := v.catalog.ResolveVersionForDate(ctx, line.ServiceDate)
	if err != nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "serviceDate", Message: "could not resolve a price catalogue for that service date"})
		return
	}
	if ver == nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "serviceDate", Message: fmt.Sprintf("no NDIS price catalogue is in effect for service date %s", line.ServiceDate)})
		return
	}

	// Step 2: find the support item by code within that version; snapshot.
	item, err := v.catalog.GetSupportItemByCode(ctx, ver.ID, line.Code)
	if err != nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "code", Message: "could not look up that support item code"})
		return
	}
	if item == nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "code", Message: fmt.Sprintf("support item code %q is not in the %s price catalogue", line.Code, ver.Label)})
		return
	}
	snapshotSupportItem(line, ver.ID, item)

	// Step 3 + 4: zone price-cap assertion (skipped when the cap is NULL).
	v.assertPriceCap(ctx, idx, ver.ID, zone, line, ve)

	// Step 5: service date within the participant plan window.
	assertPlanWindow(idx, planStart, planEnd, line.ServiceDate, ve)
}

// assertPriceCap looks up the tenant-zone price for the line's code and asserts
// unit_price ≤ price_cap (spec §6 step 4). A nil cap (quotable item) skips the
// assertion. A missing price row is itself a failure.
func (v *LineValidator) assertPriceCap(ctx context.Context, idx int, versionID int64, zone string, line *repository.LineItemInput, ve *ValidationError) {
	price, err := v.catalog.ResolveZonePrice(ctx, versionID, line.Code, zone)
	if err != nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "unitPrice", Message: "could not look up the price cap for your zone"})
		return
	}
	if price == nil {
		ve.Errors = append(ve.Errors, FieldError{Line: idx, Field: "unitPrice", Message: fmt.Sprintf("no price is published for code %q in zone %q", line.Code, zone)})
		return
	}
	if price.PriceCap == nil {
		return // quotable item: no fixed cap (spec §6 step 4).
	}
	priceCap := *price.PriceCap
	if round2(line.UnitPrice) > round2(priceCap) {
		ve.Errors = append(ve.Errors, FieldError{
			Line:    idx,
			Field:   "unitPrice",
			Message: fmt.Sprintf("unit price %.2f exceeds the NDIS price cap of %.2f for zone %q", line.UnitPrice, priceCap, zone),
		})
	}
}

// assertPlanWindow asserts serviceDate ∈ [planStart, planEnd] (spec §6 step 5).
// Empty bounds are treated as open (the participant has no recorded plan dates),
// which is permissive by design — plan-date capture is a participant concern.
func assertPlanWindow(idx int, planStart, planEnd, serviceDate string, ve *ValidationError) {
	if planStart != "" && serviceDate < planStart {
		ve.Errors = append(ve.Errors, FieldError{
			Line:    idx,
			Field:   "serviceDate",
			Message: fmt.Sprintf("service date %s is before the participant's plan start %s", serviceDate, planStart),
		})
	}
	if planEnd != "" && serviceDate > planEnd {
		ve.Errors = append(ve.Errors, FieldError{
			Line:    idx,
			Field:   "serviceDate",
			Message: fmt.Sprintf("service date %s is after the participant's plan end %s", serviceDate, planEnd),
		})
	}
}

// snapshotSupportItem pins the resolved version and snapshots the support item's
// identity onto the line (spec §6 step 2 + step 6). The description is filled
// from the item name only when the caller left it blank; gst_free is defaulted
// from the item only when the caller did not explicitly set it true.
func snapshotSupportItem(line *repository.LineItemInput, versionID int64, item *repository.SupportItem) {
	id := item.ID
	line.SupportItemID = &id
	vid := versionID
	line.CatalogVersionID = &vid
	line.Code = item.Code
	if line.Description == "" {
		line.Description = item.Name
	}
	if !line.GstFree {
		line.GstFree = item.GstFree
	}
}

// isSupportItemLine reports whether a line is an NDIS support-item line (it
// carries a code and is not a custom item). Custom-item lines carry a
// CustomItemID and no catalogue code.
func isSupportItemLine(line *repository.LineItemInput) bool {
	if line.CustomItemID != nil {
		return false
	}
	return strings.TrimSpace(line.Code) != ""
}

// computeLineTax sums round2(line_total * rate) over the non-gst-free lines,
// where line_total = round2(qty*unitPrice) — matching the repository's own
// rounding so the engine's tax agrees with the persisted subtotal. gst_free
// lines contribute zero. The total is rounded to the cent (spec §6 money note).
func computeLineTax(items []repository.LineItemInput, rate float64) float64 {
	if rate <= 0 {
		return 0
	}
	var tax float64
	for i := range items { // bounded by len(items)
		if items[i].GstFree {
			continue
		}
		lineTotal := round2(items[i].Quantity * items[i].UnitPrice)
		tax += round2(lineTotal * rate)
	}
	return round2(tax)
}

// round2 rounds to two decimal places (cents). Mirrors the repository helper of
// the same name; duplicated here to keep the engine's money rounding consistent
// without a cross-package dependency on an unexported repo function.
func round2(x float64) float64 {
	return math.Round(x*100) / 100
}

// tenantZone reads the tenant's configured NDIS zone, defaulting to "national"
// when no business profile exists yet.
func (v *LineValidator) tenantZone(ctx context.Context, tenantID int64) (string, error) {
	bp, err := v.profiles.Get(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("validate lines: read business zone: %w", err)
	}
	if bp == nil || bp.Zone == "" {
		return "national", nil
	}
	return bp.Zone, nil
}

// planWindow reads the participant's plan window. A missing participant is a
// caller error (the repository would reject the write anyway).
func (v *LineValidator) planWindow(ctx context.Context, tenantID, participantID int64) (start, end string, err error) {
	p, err := v.participants.Get(ctx, tenantID, participantID)
	if err != nil {
		return "", "", fmt.Errorf("validate lines: read participant: %w", err)
	}
	if p == nil {
		return "", "", fmt.Errorf("validate lines: participant %d not found", participantID)
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
