package payer

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
)

func newPayerSvc(t *testing.T) (*Service, *realtime.Hub, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	hub := realtime.NewHub()
	return NewService(conn, hub), hub, tenantID
}

func TestPayerCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newPayerSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	pm, err := svc.Create(tctx(tenantID), PayerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if pm == nil {
		t.Fatal("Create returned nil payer")
	}

	select {
	case e := <-ch:
		if e.Entity != "payer" || e.UUID != pm.UUID || e.Action != "create" {
			t.Fatalf("event=%+v want payer/%d/create", e, pm.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestPayerCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newPayerSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), PayerInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
