package client

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
)

func newClientSvc(t *testing.T) (*Service, *realtime.Hub, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	hub := realtime.NewHub()
	return NewService(conn, hub), hub, tenantID
}

func TestClientCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newClientSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	c, err := svc.Create(tctx(tenantID), ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c == nil {
		t.Fatal("Create returned nil client")
	}

	select {
	case e := <-ch:
		if e.Entity != "client" || e.UUID != c.UUID || e.Action != "create" {
			t.Fatalf("event=%+v want client/%d/create", e, c.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestClientCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newClientSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), ClientInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

func TestClientBulkDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID := newClientSvc(t)
	ctx := tctx(tenantID)

	c, err := svc.Create(ctx, ClientInput{Name: "Acme"})
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
		if e.Entity != "client" || e.UUID != "" || e.Action != "bulk_delete" {
			t.Fatalf("event=%+v want client/0/bulk_delete", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after BulkDelete")
	}
}
