package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/realtime"
)

func newParticipantSvc(t *testing.T) (*participant.Service, *realtime.Hub, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	hub := realtime.NewHub()
	return participant.NewService(conn, hub), hub, tenantID
}

func TestParticipantCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newParticipantSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	c, err := svc.Create(tctx(tenantID), participant.ParticipantInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c == nil {
		t.Fatal("Create returned nil participant")
	}

	select {
	case e := <-ch:
		if e.Entity != "participant" || e.ID != c.ID || e.Action != "create" {
			t.Fatalf("event=%+v want participant/%d/create", e, c.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestParticipantCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newParticipantSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), participant.ParticipantInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

func TestParticipantBulkDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID := newParticipantSvc(t)
	ctx := tctx(tenantID)

	c, err := svc.Create(ctx, participant.ParticipantInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.BulkDelete(ctx, []int64{c.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "participant" || e.ID != 0 || e.Action != "bulk_delete" {
			t.Fatalf("event=%+v want participant/0/bulk_delete", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after BulkDelete")
	}
}
