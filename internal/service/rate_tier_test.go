package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newRTSvc(t *testing.T) (*RateTierService, *realtime.Hub) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "rt.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewRateTierService(conn, hub), hub
}

func TestRateTierCreateBroadcasts(t *testing.T) {
	svc, hub := newRTSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := context.Background()

	tier, err := svc.Create(ctx, repository.RateTierInput{Name: "Std"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tier == nil {
		t.Fatal("Create returned nil tier")
	}

	select {
	case e := <-ch:
		if e.Entity != "rate_tier" || e.ID != tier.ID || e.Action != "create" {
			t.Fatalf("event=%+v want rate_tier/%d/create", e, tier.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestRateTierCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub := newRTSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(context.Background(), repository.RateTierInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

func TestRateTierDeleteLastTierNoEvent(t *testing.T) {
	svc, hub := newRTSvc(t)
	ctx := context.Background()

	ch, unsub := hub.Subscribe()
	defer unsub()

	tier, err := svc.Create(ctx, repository.RateTierInput{Name: "Only"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// drain the create event
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("no create event to drain")
	}

	if err := svc.Delete(ctx, tier.ID); !errors.Is(err, repository.ErrLastTier) {
		t.Fatalf("Delete err=%v want ErrLastTier", err)
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on last-tier delete, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
