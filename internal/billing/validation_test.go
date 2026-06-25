package billing

// Tests for the line validation engine under the catalogue model. A catalogue
// line carries a catalogueItemId; the validator reads that exact version row,
// snapshots code/description/taxable, and fills the unit price from the
// catalogue item's unit_price when the caller supplies none. A free-text line
// (no catalogueItemId) runs only the non-negativity checks. Tax is computed
// from the taxable lines at the tenant default rate.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dknathalage/tallyo/internal/catalogue"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/taxrate"
)

// --- test seeders ---------------------------------------------------------

// seedCatalogueItem inserts one catalogue item and returns its version-row uuid
// (the value a line pins via catalogueItemId).
func seedCatalogueItem(t *testing.T, conn *sql.DB, tenantID, code string, taxable bool, unitPrice float64) string {
	t.Helper()
	it, err := catalogue.NewRepo(conn).Create(context.Background(), tenantID, catalogue.CatalogueItemInput{
		Code: code, Name: "Item " + code, UnitPrice: unitPrice, Taxable: taxable,
	})
	if err != nil {
		t.Fatalf("seedCatalogueItem %s: %v", code, err)
	}
	return it.ID
}

// seedClient inserts a name-only client and returns its id.
func seedClient(t *testing.T, conn *sql.DB, tenantID string) string {
	t.Helper()
	p, err := client.NewClients(conn).Create(tctx(tenantID), tenantID, client.ClientInput{Name: "Test Client"})
	if err != nil {
		t.Fatalf("seedClient: %v", err)
	}
	return p.ID
}

// catalogueLine builds a catalogue line input pinned to a catalogue item uuid.
func catalogueLine(itemID string, qty, unitPrice float64) LineItemInput {
	id := itemID
	return LineItemInput{CatalogueItemID: &id, Quantity: qty, UnitPrice: unitPrice}
}

// --- unknown catalogue item ------------------------------------------------

func TestValidateUnknownCatalogueItemRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		catalogueLine("11111111-1111-1111-1111-111111111111", 1, 1),
	})
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Errors) != 1 || ve.Errors[0].Field != "catalogueItemId" {
		t.Fatalf("unknown item: want one catalogueItemId field error, got %v (%T)", err, err)
	}
}

// --- unit_price fill -------------------------------------------------------

func TestCatalogueLinePricesFromUnitPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	id := seedCatalogueItem(t, conn, tid, "W1", false, 9.99)
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{catalogueLine(id, 2, 0)})
	if err != nil {
		t.Fatalf("catalogue line must price from unit_price: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].UnitPrice != 9.99 {
		t.Fatalf("unit price = %v, want 9.99 (from catalogue unit_price)", res.Items[0].UnitPrice)
	}
	if res.Items[0].CatalogueItemID == nil || *res.Items[0].CatalogueItemID != id {
		t.Fatalf("catalogue item id must be pinned, got %v", res.Items[0].CatalogueItemID)
	}
}

func TestCatalogueLineKeepsCallerPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	id := seedCatalogueItem(t, conn, tid, "W1", false, 9.99)
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{catalogueLine(id, 1, 25)})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Items[0].UnitPrice != 25 {
		t.Fatalf("unit price = %v, want 25 (caller price kept)", res.Items[0].UnitPrice)
	}
}

// --- taxable defaulting (catalogue is authoritative) -----------------------

func TestValidateTaxableDefaultedFromItem(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	id := seedCatalogueItem(t, conn, tid, "GF", false, 100) // not taxable
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{catalogueLine(id, 1, 50)})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Items[0].Taxable {
		t.Fatal("taxable should be false for a non-taxable catalogue item")
	}
}

func TestValidateTaxableSetWhenItemTaxable(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	id := seedCatalogueItem(t, conn, tid, "TX", true, 100)
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{catalogueLine(id, 1, 50)})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !res.Items[0].Taxable {
		t.Fatal("taxable should be true for a taxable catalogue item")
	}
}

// The catalogue is authoritative for a catalogue line's tax status: a client
// that sends taxable:false on a taxable item must be ignored.
func TestValidateClientTaxableOverrideIgnored(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{Name: "GST", Rate: 0.10, IsDefault: true}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	id := seedCatalogueItem(t, conn, tid, "TX", true, 1000)
	v := NewLineValidator(conn)

	line := catalogueLine(id, 1, 200)
	line.Taxable = false // client lies
	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{line})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !res.Items[0].Taxable {
		t.Fatal("a taxable catalogue item must stay taxable despite client taxable:false")
	}
	if res.Tax != 20 {
		t.Fatalf("tax = %v, want 20 (override must NOT zero the tax)", res.Tax)
	}
}

func TestValidateCatalogueLineNegativeRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	id := seedCatalogueItem(t, conn, tid, "TX", true, 100)
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []LineItemInput{catalogueLine(id, -1, -5)})
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

// --- free-text (non-catalogue) path ----------------------------------------

func TestValidateFreeTextLineSkipsCatalogueChecks(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	v := NewLineValidator(conn) // no catalogue seeded

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		{Description: "Mileage", Quantity: 3, UnitPrice: 0.85},
	})
	if err != nil {
		t.Fatalf("free-text line should skip catalogue checks: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].UnitPrice != 0.85 {
		t.Fatalf("free-text line = %+v", res.Items[0])
	}
}

func TestValidateFreeTextNegativeRejected(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		{Description: "Bad", Quantity: -1, UnitPrice: -5},
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
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{Name: "GST", Rate: 0.10, IsDefault: true}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	gf := seedCatalogueItem(t, conn, tid, "GF", false, 1000) // not taxable
	tx := seedCatalogueItem(t, conn, tid, "TAX", true, 1000) // taxable
	v := NewLineValidator(conn)

	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		catalogueLine(gf, 1, 100), // not taxable -> 0 tax
		catalogueLine(tx, 1, 200), // taxable -> 20 tax
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Tax != 20 {
		t.Fatalf("tax = %v, want 20 (only the taxable 200-line at 10%%)", res.Tax)
	}
}

func TestValidateTotalsRoundToCents(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	if _, err := taxrate.NewTaxRates(conn).Create(tctx(tid), tid, taxrate.TaxRateInput{Name: "GST", Rate: 0.10, IsDefault: true}); err != nil {
		t.Fatalf("seed tax rate: %v", err)
	}
	id := seedCatalogueItem(t, conn, tid, "DRIFT", true, 1)
	v := NewLineValidator(conn)

	// 0.1 * 3 = 0.30000000000000004 in float; round2 must collapse it to 0.30.
	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{catalogueLine(id, 3, 0.1)})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Tax != 0.03 {
		t.Fatalf("tax = %v, want 0.03 (rounded)", res.Tax)
	}
}

// --- error shape across multiple lines ------------------------------------

func TestValidateAccumulatesErrorsAcrossLines(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)
	id := seedCatalogueItem(t, conn, tid, "OK", true, 100)
	v := NewLineValidator(conn)

	_, err := v.Validate(context.Background(), tid, pid, []LineItemInput{
		catalogueLine("22222222-2222-2222-2222-222222222222", 1, 1), // line 0: unknown item
		catalogueLine(id, -1, 50),                                   // line 1: negative quantity
		{Description: "free", Quantity: -2, UnitPrice: -1},          // line 2: negative qty + price
	})
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("want *ValidationError, got %T", err)
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

func TestLineValidatorReadsCatalogueFromTenant(t *testing.T) {
	tenant := newTestDB(t)
	tid := seedTenant(t, tenant)
	pid := seedClient(t, tenant, tid)
	id := seedCatalogueItem(t, tenant, tid, "PROD1", true, 49.95)

	v := NewLineValidator(tenant)
	res, err := v.Validate(context.Background(), tid, pid, []LineItemInput{catalogueLine(id, 2, 0)})
	if err != nil {
		t.Fatalf("catalogue line must resolve from the TENANT catalogue: %v", err)
	}
	if res == nil || len(res.Items) != 1 || res.Items[0].UnitPrice != 49.95 {
		t.Fatalf("unit price filled from catalogue: %+v", res)
	}
}
