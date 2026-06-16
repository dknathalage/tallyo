package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newPlanManagerSvc(t *testing.T) (*PlanManagerService, *realtime.Hub, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	hub := realtime.NewHub()
	return NewPlanManagerService(conn, hub), hub, tenantID
}

func TestPlanManagerCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newPlanManagerSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	pm, err := svc.Create(tctx(tenantID), repository.PlanManagerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if pm == nil {
		t.Fatal("Create returned nil plan manager")
	}

	select {
	case e := <-ch:
		if e.Entity != "plan_manager" || e.ID != pm.ID || e.Action != "create" {
			t.Fatalf("event=%+v want plan_manager/%d/create", e, pm.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestPlanManagerCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newPlanManagerSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), repository.PlanManagerInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
