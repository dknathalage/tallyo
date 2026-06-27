package client

import (
	"testing"
)

func newClientSvc(t *testing.T) (*Service, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	return NewService(conn), tenantID
}

func TestClientCreate(t *testing.T) {
	svc, tenantID := newClientSvc(t)

	c, err := svc.Create(tctx(tenantID), ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c == nil {
		t.Fatal("Create returned nil client")
	}
}

func TestClientCreateEmptyNameErrors(t *testing.T) {
	svc, tenantID := newClientSvc(t)

	if _, err := svc.Create(tctx(tenantID), ClientInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
}

func TestClientBulkDeleteViaService(t *testing.T) {
	svc, tenantID := newClientSvc(t)
	ctx := tctx(tenantID)

	c, err := svc.Create(ctx, ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.BulkDelete(ctx, []string{c.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
}
