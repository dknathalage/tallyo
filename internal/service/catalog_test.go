package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newCustomItemSvc(t *testing.T) (*CustomItemService, *realtime.Hub, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	hub := realtime.NewHub()
	return NewCustomItemService(conn, hub), hub, tenantID
}

func TestCustomItemCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newCustomItemSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	item, err := svc.Create(tctx(tenantID), repository.CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if item == nil {
		t.Fatal("Create returned nil item")
	}

	select {
	case e := <-ch:
		if e.Entity != "custom_item" || e.ID != item.ID || e.Action != "create" {
			t.Fatalf("event=%+v want custom_item/%d/create", e, item.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestCustomItemCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newCustomItemSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), repository.CustomItemInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

func TestCustomItemBulkDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID := newCustomItemSvc(t)
	ctx := tctx(tenantID)

	item, err := svc.Create(ctx, repository.CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe()
	defer unsub()

	if err := svc.BulkDelete(ctx, []int64{item.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "custom_item" || e.ID != 0 || e.Action != "bulk_delete" {
			t.Fatalf("event=%+v want custom_item/0/bulk_delete", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after BulkDelete")
	}
}

// SupportCatalogService is global, read-only reference data. With no catalogue
// ingested, ListVersions returns an empty (non-nil) slice.
func TestSupportCatalogListVersionsEmpty(t *testing.T) {
	conn := newTestDB(t)
	svc := NewSupportCatalogService(conn)

	versions, err := svc.ListVersions(tctx(seedTenant(t, conn)))
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("ListVersions = %d, want 0", len(versions))
	}
}
