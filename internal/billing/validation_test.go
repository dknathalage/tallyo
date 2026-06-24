package billing

// Tests for the line validation engine. Catalogue lines resolve a version by
// service date, snapshot the item, fill the unit price from the item's generic
// unit_price (when the caller supplies none), and default taxable from the item.
// Coverage: unknown code, version resolution by service date, unit_price fill +
// caller-price override, taxable defaulting, the custom-item path, tax
// computation, totals rounding, and the field-level error shape.
// (The plan-window step and the zone/price-cap path were removed.)

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/taxrate"
)

// --- test seeders ---------------------------------------------------------

// seedUnitPricedItem inserts a catalog version with one support item that has a
// generic unit_price set. Returns the version id.
func seedUnitPricedItem(t *testing.T, conn *sql.DB, tenantID string, label, from, to, code string, gstFree bool, unitPrice float64) string {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	now := time.Now().UTC().Format(time.RFC3339)
	var et sql.NullString
	if to != "" {
		et = sql.NullString{String: to, Valid: true}
	}
	v, err := q.CreatePriceListVersion(ctx, gen.CreatePriceListVersionParams{
		TenantID: tenantID, ID: ids.New(), Label: label, EffectiveFrom: from, EffectiveTo: et, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreatePriceListVersion: %v", err)
	}
	tx := int64(1)
	if gstFree {
		tx = 0
	}
	if _, err := q.CreateItem(ctx, gen.CreateItemParams{
		TenantID: tenantID, ID: ids.New(), PriceListVersionID: v.ID, Code: code, Name: "Item " + code,
		UnitPrice: sql.NullFloat64{Float64: unitPrice, Valid: true}, Taxable: tx,
	}); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	return v.ID
}

// addUnitPricedItemToVersion adds one more support item (with a generic
// unit_price) to an existing version.
func addUnitPricedItemToVersion(t *testing.T, conn *sql.DB, tenantID, versionID string, code string, gstFree bool, unitPrice float64) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	tx := int64(1)
	if gstFree {
		tx = 0
	}
	if _, err := q.CreateItem(ctx, gen.CreateItemParams{
		TenantID: tenantID, ID: ids.New(), PriceListVersionID: versionID, Code: code, Name: "Item " + code,
		UnitPrice: sql.NullFloat64{Float64: unitPrice, Valid: true}, Taxable: tx,
	}); err != nil {
		t.Fatalf("CreateItem %s: %v", code, err)
	}
}

// seedClient inserts a name-only client and returns its int PK.
func seedClient(t *testing.T, conn *sql.DB, tenantID string) string {
	t.Helper()
	p, err := client.NewClients(conn).Create(tctx(tenantID), tenantID, client.ClientInput{
		Name: "Test Client",
	})
	if err != nil {
		t.Fatalf("seedClient: %v", err)
	}
	return p.ID
}

// supportLine builds a support-item line input for the given code/date/price.
func supportLine(code, date string, qty, unitPrice float64) LineItemInput {
	return LineItemInput{Code: code, ServiceDate: date, Quantity: qty, UnitPrice: unitPrice}
}

// --- unknown code / version resolution ------------------------------------

func TestValidateUnknownCodeRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "01_011", true, 100)
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("NOPE", "2026-01-15", 1, 1),
	})
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Errors) != 1 || ve.Errors[0].Field != "code" {
		t.Fatalf("unknown code: want one code field error, got %v (%T)", err, err)
	}
}

func TestValidateNoVersionForDateRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "01_011", true, 100)
	v := NewLineValidator(conn)

	// Service date before any catalogue window.
	_, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_011", "2025-01-01", 1, 50),
	})
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Errors) != 1 || ve.Errors[0].Field != "serviceDate" {
		t.Fatalf("no version: want one serviceDate field error, got %v (%T)", err, err)
	}
}

func TestValidateVersionResolutionPicksRightVersion(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	// Two consecutive versions; the SAME code carries a different unit_price.
	seedUnitPricedItem(t, conn, tid, "2024-25", "2024-07-01", "2025-06-30", "01_011", true, 90)
	seedUnitPricedItem(t, conn, tid, "2025-26", "2025-07-01", "2026-06-30", "01_011", true, 110)
	v := NewLineValidator(conn)
	ctx := context.Background()

	// Caller supplies no price → fill from the version resolved for the date.
	res, err := v.Validate(ctx, tid, pid, []LineItemInput{supportLine("01_011", "2024-12-01", 1, 0)})
	if err != nil {
		t.Fatalf("date in 2024-25 window must resolve: %v", err)
	}
	if res.Items[0].UnitPrice != 90 {
		t.Fatalf("unit price = %v, want 90 (2024-25 version)", res.Items[0].UnitPrice)
	}
	res, err = v.Validate(ctx, tid, pid, []LineItemInput{supportLine("01_011", "2025-12-01", 1, 0)})
	if err != nil {
		t.Fatalf("date in 2025-26 window must resolve: %v", err)
	}
	if res.Items[0].UnitPrice != 110 {
		t.Fatalf("unit price = %v, want 110 (2025-26 version)", res.Items[0].UnitPrice)
	}
}

func TestValidateVersionBoundaryDatesInclusive(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "01_011", true, 100)
	v := NewLineValidator(conn)
	ctx := context.Background()

	for _, d := range []string{"2025-07-01", "2026-06-30"} { // both window boundaries
		if _, err := v.Validate(ctx, tid, pid, []LineItemInput{supportLine("01_011", d, 1, 100)}); err != nil {
			t.Fatalf("boundary date %s should resolve & pass: %v", d, err)
		}
	}
}

// --- unit_price fill -------------------------------------------------------

// TestGenericCodedLinePricesFromItemUnitPrice asserts a coded line referencing
// an item that carries a generic unit_price is priced FROM that unit_price when
// the caller supplies none (unitPrice <= 0).
func TestGenericCodedLinePricesFromItemUnitPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	// Item W1 has unit_price 9.99; taxable.
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "W1", false, 9.99)
	v := NewLineValidator(conn)
	ctx := context.Background()

	// Caller supplies unitPrice 0 → the catalogue unit_price (9.99) is applied.
	res, err := v.Validate(ctx, tid, pid, []LineItemInput{supportLine("W1", "2026-01-15", 2, 0)})
	if err != nil {
		t.Fatalf("generic coded line must price from item unit_price: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("res items = %d, want 1", len(res.Items))
	}
	if res.Items[0].UnitPrice != 9.99 {
		t.Fatalf("unit price = %v, want 9.99 (from item unit_price)", res.Items[0].UnitPrice)
	}
	if res.Items[0].ItemID == nil {
		t.Fatal("snapshot must pin item id")
	}
}

// TestGenericCodedLineKeepsCallerPriceOverUnitPrice asserts that when the caller
// supplies a positive unit price it is KEPT (the item unit_price is only a fill
// default for coded lines).
func TestGenericCodedLineKeepsCallerPriceOverUnitPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "W1", false, 9.99)
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{supportLine("W1", "2026-01-15", 1, 25)})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Items[0].UnitPrice != 25 {
		t.Fatalf("unit price = %v, want 25 (caller price kept)", res.Items[0].UnitPrice)
	}
}

// --- taxable defaulting ----------------------------------------------------

func TestValidateTaxableDefaultedFromItem(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "01_011", true, 100)
	v := NewLineValidator(conn)

	// Line leaves taxable false; the item is GST-free so taxable stays false.
	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_011", "2026-01-15", 1, 50),
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Items[0].Taxable {
		t.Fatal("taxable should be false for a GST-free support item")
	}
	if res.Items[0].ItemID == nil || res.Items[0].PriceListVersionID == nil {
		t.Fatal("item id + price-list version should be pinned (snapshot)")
	}
}

func TestValidateTaxableSetWhenItemTaxable(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "02_022", false, 100)
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("02_022", "2026-01-15", 1, 50),
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !res.Items[0].Taxable {
		t.Fatal("taxable should be true for a taxable item")
	}
}

// TestValidateClientTaxableOverrideIgnoredForSupportItem guards the compliance
// fix: the catalogue is authoritative for a support item's tax status, so a
// client that sends taxable:false on a TAXABLE catalogue item must be ignored —
// the line ends up taxable AND contributes tax.
func TestValidateClientTaxableOverrideIgnoredForSupportItem(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{
		Name: "GST", Rate: 0.10, IsDefault: true,
	}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "02_022", false, 1000)
	v := NewLineValidator(conn)

	// Client lies: taxable:false on a taxable catalogue item.
	line := supportLine("02_022", "2026-01-15", 1, 200)
	line.Taxable = false
	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{line})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !res.Items[0].Taxable {
		t.Fatal("catalogue is authoritative: a taxable item must stay taxable despite client taxable:false")
	}
	if res.Tax != 20 {
		t.Fatalf("tax = %v, want 20 (the override must NOT zero the tax)", res.Tax)
	}
}

// TestValidateSupportItemNegativeRejected exercises the non-negativity checks on
// the support-item path (the custom-item path is covered separately).
func TestValidateSupportItemNegativeRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "01_011", true, 100)
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_011", "2026-01-15", -1, -5),
	})
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("want *ValidationError, got %T: %v", err, err)
	}
	var sawQty, sawPrice bool
	for _, fe := range ve.Errors {
		if fe.Field == "quantity" {
			sawQty = true
		}
		if fe.Field == "unitPrice" {
			sawPrice = true
		}
	}
	if !sawQty || !sawPrice {
		t.Fatalf("want negative quantity AND unitPrice errors, got %+v", ve.Errors)
	}
}

// --- custom-item path ------------------------------------------------------

func TestValidateCustomItemSkipsCatalogChecks(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	// No catalogue seeded at all.
	v := NewLineValidator(conn)
	cid := "11111111-1111-1111-1111-111111111111"

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		{CustomItemID: &cid, Description: "Mileage", Quantity: 3, UnitPrice: 0.85},
	})
	if err != nil {
		t.Fatalf("custom item should skip catalogue checks: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("res items = %d", len(res.Items))
	}
}

func TestValidateCustomItemNegativeRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	v := NewLineValidator(conn)
	cid := "11111111-1111-1111-1111-111111111111"

	_, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		{CustomItemID: &cid, Description: "Bad", Quantity: -1, UnitPrice: -5},
	})
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Errors) != 2 {
		t.Fatalf("negative qty+price: want two field errors, got %v (%T)", err, err)
	}
}

// --- tax computation + totals rounding ------------------------------------

func TestValidateComputesTaxFromTaxableLines(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	// Default 10% GST.
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{
		Name: "GST", Rate: 0.10, IsDefault: true,
	}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	// One version carrying both a GST-free item (0 tax) and a taxable item.
	verID := seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "GF", true, 1000)
	addUnitPricedItemToVersion(t, conn, tid, verID, "TAX", false, 1000)
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("GF", "2026-01-15", 1, 100),  // gst-free → 0 tax
		supportLine("TAX", "2026-01-15", 1, 200), // taxable → 20 tax
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Tax != 20 {
		t.Fatalf("tax = %v, want 20 (only the taxable 200-line at 10%%)", res.Tax)
	}
}

// TestCharacterizeMixedTaxMath pins the tax math on a mixed invoice: a gst_free
// line contributes NO tax, and a taxable line is taxed at the default rate. The
// computed tax must equal ONLY the taxed line's Round2(lineTotal*rate).
func TestCharacterizeMixedTaxMath(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	const rate = 0.10
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{
		Name: "GST", Rate: rate, IsDefault: true,
	}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	// One version carrying a gst_free item (no tax) and a taxable item (taxed).
	verID := seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "GF", true, 1000)
	addUnitPricedItemToVersion(t, conn, tid, verID, "TAX", false, 1000)
	v := NewLineValidator(conn)

	const taxedQty, taxedPrice = 1.0, 200.0
	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("GF", "2026-01-15", 1, 100),                // gst-free → 0 tax
		supportLine("TAX", "2026-01-15", taxedQty, taxedPrice), // taxable → taxed
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	wantTax := Round2(Round2(taxedQty*taxedPrice) * rate)
	if res.Tax != wantTax {
		t.Fatalf("tax = %v, want %v (only the taxable line)", res.Tax, wantTax)
	}
}

func TestValidateTotalsRoundToCents(t *testing.T) {
	// 0.1 * 3 = 0.30000000000000004 in float; round2 must collapse it to 0.30.
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{
		Name: "GST", Rate: 0.10, IsDefault: true,
	}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "DRIFT", false, 1)
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("DRIFT", "2026-01-15", 3, 0.1),
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// line_total = round2(3*0.1)=0.30; tax = round2(0.30*0.10)=0.03.
	if res.Tax != 0.03 {
		t.Fatalf("tax = %v, want 0.03 (rounded)", res.Tax)
	}
}

// --- error shape across multiple lines ------------------------------------

func TestValidateAccumulatesErrorsAcrossLines(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-01-01", "2026-12-31", "01_011", true, 100)
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("01_011", "", 1, 200),           // line 0: missing service date
		supportLine("NOPE", "2025-08-01", 1, 1),     // line 1: unknown code
		supportLine("01_011", "2026-06-01", -1, 50), // line 2: negative quantity
	})
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("want *ValidationError, got %T", err)
	}
	if len(ve.Errors) != 3 {
		t.Fatalf("want 3 accumulated errors, got %d: %+v", len(ve.Errors), ve.Errors)
	}
	gotLines := map[int]bool{}
	for _, fe := range ve.Errors {
		gotLines[fe.Line] = true
	}
	for _, ln := range []int{0, 1, 2} {
		if !gotLines[ln] {
			t.Fatalf("missing error for line %d: %+v", ln, ve.Errors)
		}
	}
}

// TestLineValidatorReadsCatalogueFromTenant is the regression guard for the
// control/tenant split: the price list is tenant-owned, so the validator's
// catalogue repo MUST read from the TENANT db.
func TestLineValidatorReadsCatalogueFromTenant(t *testing.T) {
	tenant := newTestDB(t)
	tid := seedTenant(t, tenant)
	pid := seedClient(t, tenant, tid)
	seedUnitPricedItem(t, tenant, tid, "v1", "2025-07-01", "2026-06-30", "PROD1", true, 49.95)

	v := NewLineValidator(tenant)
	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		supportLine("PROD1", "2026-01-15", 2, 0),
	})
	if err != nil {
		t.Fatalf("coded line must resolve from the TENANT price list: %v", err)
	}
	if res == nil || len(res.Items) != 1 {
		t.Fatalf("res = %+v", res)
	}
	if got := res.Items[0].UnitPrice; got != 49.95 {
		t.Fatalf("unit price filled from items.unit_price: got %v want 49.95", got)
	}
}
