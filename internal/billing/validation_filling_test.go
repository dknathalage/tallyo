package billing

// Tests for ValidateFilling (now a thin alias for Validate — both fill a
// catalogue line's price from the item's generic unit_price) plus the
// ValidationError rendering / unwrap helpers.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dknathalage/tallyo/internal/taxrate"
)

// --- ValidateFilling: unit_price fill --------------------------------------

// TestValidateFillingFillsFromItemUnitPrice proves fill mode prices a coded line
// from the item's generic unit_price when the caller supplies none.
func TestValidateFillingFillsFromItemUnitPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, 100)
	v := NewLineValidator(conn)

	res, err := v.ValidateFilling(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_011", "2026-01-15", 2, 0),
	})
	if err != nil {
		t.Fatalf("ValidateFilling: %v", err)
	}
	if res.Items[0].UnitPrice != 100 {
		t.Fatalf("unit price = %v, want 100 (filled from item unit_price)", res.Items[0].UnitPrice)
	}
}

// TestValidateFillingKeepsPositiveCallerPrice proves a positive caller price is
// kept (the item unit_price is only a fill default).
func TestValidateFillingKeepsPositiveCallerPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, 100)
	v := NewLineValidator(conn)

	res, err := v.ValidateFilling(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_011", "2026-01-15", 1, 250),
	})
	if err != nil {
		t.Fatalf("ValidateFilling: %v", err)
	}
	if res.Items[0].UnitPrice != 250 {
		t.Fatalf("unit price = %v, want 250 (caller price kept)", res.Items[0].UnitPrice)
	}
}

// TestValidateFillingComputesTaxOnFilledPrice proves the engine-computed tax in
// fill mode is derived from the filled unit_price.
func TestValidateFillingComputesTaxOnFilledPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{
		Name: "GST", Rate: 0.10, IsDefault: true,
	}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	// Taxable item, unit_price 200; caller sends 0 → filled to 200.
	seedUnitPricedItem(t, conn, "v1", "2025-07-01", "2026-06-30", "02_022", false, 200)
	v := NewLineValidator(conn)

	res, err := v.ValidateFilling(context.Background(), tid, pid, []LineItemInput{
		supportLine("02_022", "2026-01-15", 1, 0),
	})
	if err != nil {
		t.Fatalf("ValidateFilling: %v", err)
	}
	// tax = round2(round2(1*200) * 0.10) = 20, from the filled price.
	if res.Tax != 20 {
		t.Fatalf("tax = %v, want 20 (computed from filled unit_price 200)", res.Tax)
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
		{Line: 2, Field: "unitPrice", Message: "negative"},
	}}
	got := ve.Error()
	want := "validation failed: line 0: code: unknown; line 2: unitPrice: negative"
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
