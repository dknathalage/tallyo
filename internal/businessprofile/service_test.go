package businessprofile

import (
	"testing"
)

func newSvc(t *testing.T) (*Service, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	return NewService(conn), tenantID
}

func TestSaveAndGet(t *testing.T) {
	svc, tenantID := newSvc(t)
	ctx := tctx(tenantID)

	if err := svc.Save(ctx, BusinessProfileInput{Name: "Acme"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := svc.Get(ctx)
	if err != nil || got == nil || got.Name != "Acme" {
		t.Fatalf("Get=%+v err=%v", got, err)
	}
}

func TestSaveEmptyNameErrors(t *testing.T) {
	svc, tenantID := newSvc(t)
	if err := svc.Save(tctx(tenantID), BusinessProfileInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
}
