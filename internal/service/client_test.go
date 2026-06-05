package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newClientSvc(t *testing.T) (*ClientService, *realtime.Hub) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "client.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewClientService(conn, hub), hub
}

func TestClientCreateBroadcasts(t *testing.T) {
	svc, hub := newClientSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	c, err := svc.Create(context.Background(), repository.ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c == nil {
		t.Fatal("Create returned nil client")
	}

	select {
	case e := <-ch:
		if e.Entity != "client" || e.ID != c.ID || e.Action != "create" {
			t.Fatalf("event=%+v want client/%d/create", e, c.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestClientCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub := newClientSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(context.Background(), repository.ClientInput{Name: ""}); err == nil {
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
	svc, hub := newClientSvc(t)
	ctx := context.Background()

	c, err := svc.Create(ctx, repository.ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe()
	defer unsub()

	if err := svc.BulkDelete(ctx, []int64{c.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "client" || e.ID != 0 || e.Action != "bulk_delete" {
			t.Fatalf("event=%+v want client/0/bulk_delete", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after BulkDelete")
	}
}
