package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newTaxSvc(t *testing.T) (*TaxRateService, *realtime.Hub, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	hub := realtime.NewHub()
	return NewTaxRateService(conn, hub), hub, tenantID
}

func TestTaxRateCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newTaxSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	tr, err := svc.Create(tctx(tenantID), repository.TaxRateInput{Name: "GST", Rate: 10})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tr == nil {
		t.Fatal("Create returned nil tax rate")
	}

	select {
	case e := <-ch:
		if e.Entity != "tax_rate" || e.ID != tr.ID || e.Action != "create" {
			t.Fatalf("event=%+v want tax_rate/%d/create", e, tr.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestTaxRateCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newTaxSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), repository.TaxRateInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
