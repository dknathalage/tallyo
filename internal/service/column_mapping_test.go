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

func newCMSvc(t *testing.T) (*ColumnMappingService, *realtime.Hub) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "cm.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewColumnMappingService(conn, hub), hub
}

func TestColumnMappingCreateBroadcasts(t *testing.T) {
	svc, hub := newCMSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := context.Background()

	m, err := svc.Create(ctx, repository.ColumnMappingInput{Name: "Vendor"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m == nil {
		t.Fatal("Create returned nil mapping")
	}

	select {
	case e := <-ch:
		if e.Entity != "column_mapping" || e.ID != m.ID || e.Action != "create" {
			t.Fatalf("event=%+v want column_mapping/%d/create", e, m.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestColumnMappingCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub := newCMSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(context.Background(), repository.ColumnMappingInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
