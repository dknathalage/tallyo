package payer

import (
	"testing"
)

func newPayerSvc(t *testing.T) (*Service, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	return NewService(conn), tenantID
}

func TestPayerCreate(t *testing.T) {
	svc, tenantID := newPayerSvc(t)

	pm, err := svc.Create(tctx(tenantID), PayerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if pm == nil {
		t.Fatal("Create returned nil payer")
	}
}

func TestPayerCreateEmptyNameErrors(t *testing.T) {
	svc, tenantID := newPayerSvc(t)

	if _, err := svc.Create(tctx(tenantID), PayerInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
}
