package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/businessprofile"
	"github.com/dknathalage/tallyo/internal/realtime"
)

func newSvc(t *testing.T) (*businessprofile.Service, *realtime.Hub, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	hub := realtime.NewHub()
	return businessprofile.NewService(conn, hub), hub, tenantID
}

func TestSaveBroadcastsAfterCommit(t *testing.T) {
	svc, hub, tenantID := newSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	if err := svc.Save(ctx, businessprofile.BusinessProfileInput{Name: "Acme"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := svc.Get(ctx)
	if err != nil || got == nil || got.Name != "Acme" {
		t.Fatalf("Get=%+v err=%v", got, err)
	}

	select {
	case e := <-ch:
		if e.Entity != "business_profile" || e.Action != "update" {
			t.Fatalf("event=%+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Save")
	}
}

func TestSaveEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	if err := svc.Save(tctx(tenantID), businessprofile.BusinessProfileInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed save, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
