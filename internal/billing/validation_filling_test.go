package billing

// Tests for the catalogue-authoritative fill-pricing mode (ValidateFilling, used
// by the agent create path) plus the ValidationError rendering / unwrap helpers.
// These exercise branch-new code the human-UI Validate tests do not reach.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dknathalage/tallyo/internal/taxrate"
)

// --- ValidateFilling: catalogue-authoritative pricing ---------------------

// TestValidateFillingOverwritesUnitPriceWithCap proves fill mode IGNORES the
// caller's unit price and pins the resolved zone cap onto the line, even when
// the caller supplied a price well below (or above) the cap.
func TestValidateFillingOverwritesUnitPriceWithCap(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClientPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	setTenantZone(t, conn, tid, "national")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn, conn)

	// Caller sends a nonsense price; fill mode must replace it with the cap (100).
	res, err := v.ValidateFilling(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_011", "2026-01-15", 2, 7),
	})
	if err != nil {
		t.Fatalf("ValidateFilling: %v", err)
	}
	if res.Items[0].UnitPrice != 100 {
		t.Fatalf("unit price = %v, want 100 (cap pinned)", res.Items[0].UnitPrice)
	}
}

// TestValidateFillingOverCapPriceStillPinnedToCap proves fill mode never
// rejects on price — an over-cap caller price is simply overwritten, not an
// error (the model cannot misprice a coded line).
func TestValidateFillingOverCapPriceStillPinnedToCap(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClientPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	setTenantZone(t, conn, tid, "national")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn, conn)

	res, err := v.ValidateFilling(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_011", "2026-01-15", 1, 9999),
	})
	if err != nil {
		t.Fatalf("over-cap price in fill mode must not error: %v", err)
	}
	if res.Items[0].UnitPrice != 100 {
		t.Fatalf("unit price = %v, want 100 (cap pinned, caller 9999 ignored)", res.Items[0].UnitPrice)
	}
}

// TestValidateFillingQuotableKeepsPositiveCallerPrice proves a quotable item
// (nil cap) has no price to apply, so fill mode keeps the caller's positive
// unit price.
func TestValidateFillingQuotableKeepsPositiveCallerPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClientPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	setTenantZone(t, conn, tid, "national")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_999", true, map[string]*float64{"national": nil})
	v := NewLineValidator(conn, conn)

	res, err := v.ValidateFilling(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_999", "2026-01-15", 1, 250),
	})
	if err != nil {
		t.Fatalf("quotable item with positive price should pass: %v", err)
	}
	if res.Items[0].UnitPrice != 250 {
		t.Fatalf("unit price = %v, want 250 (caller price kept for quotable)", res.Items[0].UnitPrice)
	}
}

// TestValidateFillingQuotableZeroPriceRejected proves a quotable item with no
// positive caller price is a failure (there is no published price to apply).
func TestValidateFillingQuotableZeroPriceRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClientPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	setTenantZone(t, conn, tid, "national")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_999", true, map[string]*float64{"national": nil})
	v := NewLineValidator(conn, conn)

	_, err := v.ValidateFilling(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_999", "2026-01-15", 1, 0),
	})
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Errors) != 1 || ve.Errors[0].Field != "unitPrice" {
		t.Fatalf("quotable zero price: want one unitPrice error, got %v (%T)", err, err)
	}
}

// TestValidateFillingComputesTaxOnPinnedPrice proves the engine-computed tax in
// fill mode is derived from the PINNED cap price, not the caller's price.
func TestValidateFillingComputesTaxOnPinnedPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClientPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{
		Name: "GST", Rate: 0.10, IsDefault: true,
	}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	setTenantZone(t, conn, tid, "national")
	// Taxable item, cap 200; caller sends 5 which must be ignored.
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "02_022", false, map[string]*float64{"national": fptr(200)})
	v := NewLineValidator(conn, conn)

	res, err := v.ValidateFilling(context.Background(), tid, pid, []LineItemInput{
		supportLine("02_022", "2026-01-15", 1, 5),
	})
	if err != nil {
		t.Fatalf("ValidateFilling: %v", err)
	}
	// tax = round2(round2(1*200) * 0.10) = 20, from the pinned cap not the 5.
	if res.Tax != 20 {
		t.Fatalf("tax = %v, want 20 (computed from pinned cap 200)", res.Tax)
	}
}

// --- ValidationError.Error rendering --------------------------------------

func TestValidationErrorNilRendersDefault(t *testing.T) {
	var ve *ValidationError
	if got := ve.Error(); got != "validation failed" {
		t.Fatalf("nil ValidationError.Error() = %q, want %q", got, "validation failed")
	}
}

func TestValidationErrorEmptyRendersDefault(t *testing.T) {
	ve := &ValidationError{}
	if got := ve.Error(); got != "validation failed" {
		t.Fatalf("empty ValidationError.Error() = %q, want %q", got, "validation failed")
	}
}

func TestValidationErrorRendersEveryFieldError(t *testing.T) {
	ve := &ValidationError{Errors: []FieldError{
		{Line: 0, Field: "code", Message: "unknown"},
		{Line: 2, Field: "unitPrice", Message: "over cap"},
	}}
	got := ve.Error()
	want := "validation failed: line 0: code: unknown; line 2: unitPrice: over cap"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

// --- AsValidationError unwrap ---------------------------------------------

func TestAsValidationErrorNil(t *testing.T) {
	if ve, ok := AsValidationError(nil); ok || ve != nil {
		t.Fatalf("AsValidationError(nil) = (%v, %v), want (nil, false)", ve, ok)
	}
}

func TestAsValidationErrorPlainErrorIsNotValidation(t *testing.T) {
	if ve, ok := AsValidationError(errors.New("boom")); ok || ve != nil {
		t.Fatalf("AsValidationError(plain) = (%v, %v), want (nil, false)", ve, ok)
	}
}

func TestAsValidationErrorUnwrapsWrapped(t *testing.T) {
	inner := &ValidationError{Errors: []FieldError{{Line: 1, Field: "code", Message: "x"}}}
	wrapped := fmt.Errorf("service create: %w", inner)
	ve, ok := AsValidationError(wrapped)
	if !ok || ve == nil {
		t.Fatalf("AsValidationError(wrapped) = (%v, %v), want the inner ValidationError", ve, ok)
	}
	if len(ve.Errors) != 1 || ve.Errors[0].Field != "code" {
		t.Fatalf("unwrapped errors = %+v", ve.Errors)
	}
}
