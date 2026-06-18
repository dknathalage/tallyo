package service

// Tests for the NDIS line validation engine (spec §6 / §10). This is the
// heaviest-coverage unit per the testing strategy: over-cap rejection, at-cap
// acceptance, quotable (NULL cap), unknown code, plan-window boundaries,
// version resolution by service date, zone selection, gst_free defaulting, the
// custom-item path, totals rounding, and the field-level error shape.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/taxrate"
	"github.com/google/uuid"
)

// --- test seeders ---------------------------------------------------------

// seedZonedCatalog inserts a catalog version with one support item priced per
// the given zone→cap map (a nil cap value = quotable). Returns the version id.
func seedZonedCatalog(t *testing.T, conn *sql.DB, label, from, to, code string, gstFree bool, prices map[string]*float64) int64 {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	now := time.Now().UTC().Format(time.RFC3339)
	var et sql.NullString
	if to != "" {
		et = sql.NullString{String: to, Valid: true}
	}
	v, err := q.CreateCatalogVersion(ctx, gen.CreateCatalogVersionParams{
		Uuid: uuid.NewString(), Label: label, EffectiveFrom: from, EffectiveTo: et, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateCatalogVersion: %v", err)
	}
	gf := int64(0)
	if gstFree {
		gf = 1
	}
	si, err := q.CreateSupportItem(ctx, gen.CreateSupportItemParams{
		Uuid: uuid.NewString(), CatalogVersionID: v.ID, Code: code, Name: "Item " + code, GstFree: gf,
	})
	if err != nil {
		t.Fatalf("CreateSupportItem: %v", err)
	}
	for zone, capPtr := range prices { // bounded by len(prices)
		var pc sql.NullFloat64
		if capPtr != nil {
			pc = sql.NullFloat64{Float64: *capPtr, Valid: true}
		}
		if _, err := q.CreateSupportItemPrice(ctx, gen.CreateSupportItemPriceParams{
			SupportItemID: si.ID, Zone: zone, PriceCap: pc,
		}); err != nil {
			t.Fatalf("CreateSupportItemPrice %s: %v", zone, err)
		}
	}
	return v.ID
}

// addItemToVersion adds one more priced support item to an existing version.
func addItemToVersion(t *testing.T, conn *sql.DB, versionID int64, code string, gstFree bool, prices map[string]*float64) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	gf := int64(0)
	if gstFree {
		gf = 1
	}
	si, err := q.CreateSupportItem(ctx, gen.CreateSupportItemParams{
		Uuid: uuid.NewString(), CatalogVersionID: versionID, Code: code, Name: "Item " + code, GstFree: gf,
	})
	if err != nil {
		t.Fatalf("CreateSupportItem %s: %v", code, err)
	}
	for zone, capPtr := range prices { // bounded by len(prices)
		var pc sql.NullFloat64
		if capPtr != nil {
			pc = sql.NullFloat64{Float64: *capPtr, Valid: true}
		}
		if _, err := q.CreateSupportItemPrice(ctx, gen.CreateSupportItemPriceParams{
			SupportItemID: si.ID, Zone: zone, PriceCap: pc,
		}); err != nil {
			t.Fatalf("CreateSupportItemPrice %s/%s: %v", code, zone, err)
		}
	}
}

// seedParticipantPlan inserts a participant with an explicit plan window.
func seedParticipantPlan(t *testing.T, conn *sql.DB, tenantID int64, planStart, planEnd string) int64 {
	t.Helper()
	p, err := repository.NewParticipants(conn).Create(tctx(tenantID), tenantID, repository.ParticipantInput{
		Name: "Plan Participant", PlanStart: planStart, PlanEnd: planEnd,
	})
	if err != nil {
		t.Fatalf("seedParticipantPlan: %v", err)
	}
	return p.ID
}

// setTenantZone saves a business profile with the given zone for the tenant.
func setTenantZone(t *testing.T, conn *sql.DB, tenantID int64, zone string) {
	t.Helper()
	if err := repository.NewBusinessProfile(conn).Save(tctx(tenantID), tenantID, repository.BusinessProfileInput{
		Name: "Acme NDIS", Zone: zone,
	}); err != nil {
		t.Fatalf("setTenantZone: %v", err)
	}
}

func fptr(f float64) *float64 { return &f }

// supportLine builds a support-item line input for the given code/date/price.
func supportLine(code, date string, qty, unitPrice float64) billing.LineItemInput {
	return billing.LineItemInput{Code: code, ServiceDate: date, Quantity: qty, UnitPrice: unitPrice}
}

// --- price-cap assertion (spec §6 step 4) --------------------------------

func TestValidateOverCapRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
		supportLine("01_011", "2026-01-15", 1, 100.01),
	})
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("want *ValidationError, got %T: %v", err, err)
	}
	if len(ve.Errors) != 1 || ve.Errors[0].Field != "unitPrice" || ve.Errors[0].Line != 0 {
		t.Fatalf("field error shape = %+v", ve.Errors)
	}
}

func TestValidateAtCapAllowed(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
		supportLine("01_011", "2026-01-15", 2, 100),
	})
	if err != nil {
		t.Fatalf("at-cap should pass: %v", err)
	}
	if res == nil || len(res.Items) != 1 {
		t.Fatalf("res = %+v", res)
	}
}

func TestValidateQuotableNilCapAllowsAnyPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_999", true, map[string]*float64{"national": nil})
	v := NewLineValidator(conn)

	if _, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
		supportLine("01_999", "2026-01-15", 1, 999999),
	}); err != nil {
		t.Fatalf("quotable item (nil cap) should allow any price: %v", err)
	}
}

// --- unknown code / version resolution (spec §6 steps 1-2) ----------------

func TestValidateUnknownCodeRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
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
	pid := seedParticipantPlan(t, conn, tid, "2024-01-01", "2030-01-01")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)

	// Service date before any catalogue window.
	_, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
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
	pid := seedParticipantPlan(t, conn, tid, "2024-01-01", "2030-01-01")
	// Two consecutive versions; the SAME code has different caps per version.
	seedZonedCatalog(t, conn, "2024-25", "2024-07-01", "2025-06-30", "01_011", true, map[string]*float64{"national": fptr(90)})
	seedZonedCatalog(t, conn, "2025-26", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(110)})
	v := NewLineValidator(conn)
	ctx := context.Background()

	// 100 is over the 2024-25 cap (90) but under the 2025-26 cap (110).
	if _, err := v.Validate(ctx, tid, pid, []billing.LineItemInput{supportLine("01_011", "2024-12-01", 1, 100)}); err == nil {
		t.Fatal("date in 2024-25 window: 100 > cap 90 must reject")
	}
	if _, err := v.Validate(ctx, tid, pid, []billing.LineItemInput{supportLine("01_011", "2025-12-01", 1, 100)}); err != nil {
		t.Fatalf("date in 2025-26 window: 100 < cap 110 must pass: %v", err)
	}
}

func TestValidateVersionBoundaryDatesInclusive(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2024-01-01", "2030-01-01")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)
	ctx := context.Background()

	for _, d := range []string{"2025-07-01", "2026-06-30"} { // both window boundaries
		if _, err := v.Validate(ctx, tid, pid, []billing.LineItemInput{supportLine("01_011", d, 1, 100)}); err != nil {
			t.Fatalf("boundary date %s should resolve & pass: %v", d, err)
		}
	}
}

// --- plan window (spec §6 step 5) -----------------------------------------

func TestValidateServiceDateOutsidePlanRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2025-12-31")
	seedZonedCatalog(t, conn, "v1", "2025-01-01", "2026-12-31", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)
	ctx := context.Background()

	// After plan end (date still inside a valid catalogue window).
	_, err := v.Validate(ctx, tid, pid, []billing.LineItemInput{supportLine("01_011", "2026-01-15", 1, 50)})
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Errors) != 1 || ve.Errors[0].Field != "serviceDate" {
		t.Fatalf("after plan end: want one serviceDate error, got %v (%T)", err, err)
	}
}

func TestValidateServiceDateOnPlanBoundaryAllowed(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2025-12-31")
	seedZonedCatalog(t, conn, "v1", "2025-01-01", "2026-12-31", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)
	ctx := context.Background()

	for _, d := range []string{"2025-07-01", "2025-12-31"} { // plan window boundaries
		if _, err := v.Validate(ctx, tid, pid, []billing.LineItemInput{supportLine("01_011", d, 1, 50)}); err != nil {
			t.Fatalf("plan boundary date %s should pass: %v", d, err)
		}
	}
}

// --- zone selection (spec §6 step 3) --------------------------------------

func TestValidateZoneSelectsDifferentCap(t *testing.T) {
	conn := newTestDB(t)
	pid2start, pid2end := "2025-07-01", "2026-06-30"
	// national cap 100, remote cap 150 for the same code.
	prices := map[string]*float64{"national": fptr(100), "remote": fptr(150)}

	// Tenant A: national zone → cap 100, 130 rejected.
	a := seedTenant(t, conn)
	pa := seedParticipantPlan(t, conn, a, pid2start, pid2end)
	setTenantZone(t, conn, a, "national")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, prices)
	v := NewLineValidator(conn)
	ctx := context.Background()

	if _, err := v.Validate(ctx, a, pa, []billing.LineItemInput{supportLine("01_011", "2026-01-15", 1, 130)}); err == nil {
		t.Fatal("national tenant: 130 > cap 100 must reject")
	}

	// Tenant B: remote zone → cap 150, 130 allowed.
	b := seedTenant(t, conn)
	pb := seedParticipantPlan(t, conn, b, pid2start, pid2end)
	setTenantZone(t, conn, b, "remote")
	if _, err := v.Validate(ctx, b, pb, []billing.LineItemInput{supportLine("01_011", "2026-01-15", 1, 130)}); err != nil {
		t.Fatalf("remote tenant: 130 < cap 150 must pass: %v", err)
	}
}

// TestValidateDefaultsToNationalZoneWhenNoProfile asserts the tenantZone
// fallback (validation.go): a tenant with NO business_profile (zone unset) is
// treated as "national", so caps resolve against the national price.
func TestValidateDefaultsToNationalZoneWhenNoProfile(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn) // deliberately NO setTenantZone → no business profile
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	// national cap 100; a remote price also exists at 150 to prove the fallback
	// picks national (not remote, and not "any zone").
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true,
		map[string]*float64{"national": fptr(100), "remote": fptr(150)})
	v := NewLineValidator(conn)
	ctx := context.Background()

	// 120 is over the national cap (100) but under the remote cap (150). With the
	// national fallback this must be REJECTED.
	if _, err := v.Validate(ctx, tid, pid, []billing.LineItemInput{supportLine("01_011", "2026-01-15", 1, 120)}); err == nil {
		t.Fatal("no-profile tenant: 120 > national cap 100 must reject (national fallback)")
	}
	// 100 is exactly the national cap → must pass.
	if _, err := v.Validate(ctx, tid, pid, []billing.LineItemInput{supportLine("01_011", "2026-01-15", 1, 100)}); err != nil {
		t.Fatalf("no-profile tenant: 100 == national cap must pass: %v", err)
	}
}

// --- gst_free defaulting (spec §6 step 6) ---------------------------------

func TestValidateGstFreeDefaultedFromItem(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)

	// Line leaves gstFree false; the item is GST-free so it should default true.
	res, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
		supportLine("01_011", "2026-01-15", 1, 50),
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !res.Items[0].GstFree {
		t.Fatal("gstFree should default true from the support item")
	}
	if res.Items[0].SupportItemID == nil || res.Items[0].CatalogVersionID == nil {
		t.Fatal("support item id + catalog version should be pinned (snapshot)")
	}
}

func TestValidateGstFreeNotDefaultedWhenItemTaxable(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "02_022", false, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
		supportLine("02_022", "2026-01-15", 1, 50),
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Items[0].GstFree {
		t.Fatal("gstFree should stay false for a taxable item")
	}
}

// TestValidateClientGstFreeOverrideIgnoredForSupportItem guards the compliance
// fix: the catalogue is authoritative for a support item's GST status, so a
// client that sends gstFree:true on a TAXABLE catalogue item must be ignored —
// the line ends up taxable AND contributes tax.
func TestValidateClientGstFreeOverrideIgnoredForSupportItem(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{
		Name: "GST", Rate: 0.10, IsDefault: true,
	}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "02_022", false, map[string]*float64{"national": fptr(1000)})
	v := NewLineValidator(conn)

	// Client lies: gstFree:true on a taxable catalogue item.
	line := supportLine("02_022", "2026-01-15", 1, 200)
	line.GstFree = true
	res, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{line})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Items[0].GstFree {
		t.Fatal("catalogue is authoritative: a taxable item must stay taxable despite client gstFree:true")
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
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
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

// --- custom-item path (spec §6) -------------------------------------------

func TestValidateCustomItemSkipsCatalogChecks(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	// No catalogue seeded at all.
	v := NewLineValidator(conn)
	cid := int64(1)

	res, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
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
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	v := NewLineValidator(conn)
	cid := int64(1)

	_, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
		{CustomItemID: &cid, Description: "Bad", Quantity: -1, UnitPrice: -5},
	})
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Errors) != 2 {
		t.Fatalf("negative qty+price: want two field errors, got %v (%T)", err, err)
	}
}

// --- tax computation + totals rounding ------------------------------------

func TestValidateComputesTaxFromNonGstFreeLines(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	// Default 10% GST.
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{
		Name: "GST", Rate: 0.10, IsDefault: true,
	}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	// One version carrying both a GST-free item (0 tax) and a taxable item.
	verID := seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "GF", true, map[string]*float64{"national": fptr(1000)})
	addItemToVersion(t, conn, verID, "TAX", false, map[string]*float64{"national": fptr(1000)})
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
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

func TestValidateTotalsRoundToCents(t *testing.T) {
	// 0.1 * 3 = 0.30000000000000004 in float; round2 must collapse it to 0.30.
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2026-06-30")
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{
		Name: "GST", Rate: 0.10, IsDefault: true,
	}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "DRIFT", false, map[string]*float64{"national": fptr(1)})
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
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
	pid := seedParticipantPlan(t, conn, tid, "2025-07-01", "2025-12-31")
	seedZonedCatalog(t, conn, "v1", "2025-01-01", "2026-12-31", "01_011", true, map[string]*float64{"national": fptr(100)})
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []billing.LineItemInput{
		supportLine("01_011", "2025-08-01", 1, 200), // line 0: over cap
		supportLine("NOPE", "2025-08-01", 1, 1),     // line 1: unknown code
		supportLine("01_011", "2026-06-01", 1, 50),  // line 2: after plan end
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
