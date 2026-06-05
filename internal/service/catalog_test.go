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

func newCatalogSvc(t *testing.T) (*CatalogService, *realtime.Hub, *repository.RateTiersRepo) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewCatalogService(conn, hub), hub, repository.NewRateTiers(conn)
}

func TestCatalogCreateBroadcasts(t *testing.T) {
	svc, hub, _ := newCatalogSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	item, err := svc.Create(context.Background(), repository.CatalogItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if item == nil {
		t.Fatal("Create returned nil item")
	}

	select {
	case e := <-ch:
		if e.Entity != "catalog_item" || e.ID != item.ID || e.Action != "create" {
			t.Fatalf("event=%+v want catalog_item/%d/create", e, item.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestCatalogCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, _ := newCatalogSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(context.Background(), repository.CatalogItemInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

func TestCatalogSetRateBroadcasts(t *testing.T) {
	svc, hub, tiers := newCatalogSvc(t)
	ctx := context.Background()

	item, err := svc.Create(ctx, repository.CatalogItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tier, err := tiers.Create(ctx, repository.RateTierInput{Name: "Standard", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create tier: %v", err)
	}

	ch, unsub := hub.Subscribe()
	defer unsub()

	if err := svc.SetRate(ctx, item.ID, tier.ID, 7.5); err != nil {
		t.Fatalf("SetRate: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "catalog_item" || e.ID != item.ID || e.Action != "set_rate" {
			t.Fatalf("event=%+v want catalog_item/%d/set_rate", e, item.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after SetRate")
	}
}
