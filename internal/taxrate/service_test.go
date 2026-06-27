package taxrate

import (
	"testing"
)

func newTaxSvc(t *testing.T) (*Service, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	return NewService(conn), tenantID
}

func TestTaxRateCreate(t *testing.T) {
	svc, tenantID := newTaxSvc(t)

	tr, err := svc.Create(tctx(tenantID), TaxRateInput{Name: "GST", Rate: 10})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tr == nil {
		t.Fatal("Create returned nil tax rate")
	}
}

func TestTaxRateCreateEmptyNameErrors(t *testing.T) {
	svc, tenantID := newTaxSvc(t)

	if _, err := svc.Create(tctx(tenantID), TaxRateInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
}
