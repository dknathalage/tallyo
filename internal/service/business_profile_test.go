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

func newSvc(t *testing.T) (*BusinessProfileService, *realtime.Hub) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "svc.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewBusinessProfileService(conn, hub), hub
}

func TestSaveBroadcastsAfterCommit(t *testing.T) {
	svc, hub := newSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := context.Background()

	if err := svc.Save(ctx, repository.BusinessProfileInput{Name: "Acme"}); err != nil {
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
	svc, hub := newSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()
	if err := svc.Save(context.Background(), repository.BusinessProfileInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed save, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
