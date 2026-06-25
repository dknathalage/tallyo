package customitem

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/apperr"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "customitem.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return conn
}

func seedTenant(t *testing.T, conn *sql.DB) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		ID:        ids.New(),
		Name:      "Acme",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedTenant: %v", err)
	}
	return tn.ID
}

func tctx(tenantID string) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}

func newSvc(t *testing.T) (*Service, *realtime.Hub, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	hub := realtime.NewHub()
	return NewService(conn, hub), hub, tenantID
}

func TestCustomItemCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	item, err := svc.Create(tctx(tenantID), CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if item == nil {
		t.Fatal("Create returned nil item")
	}

	select {
	case e := <-ch:
		if e.Entity != "custom_item" || e.UUID != item.ID || e.Action != "create" {
			t.Fatalf("event=%+v want custom_item/%s/create", e, item.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestCustomItemCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), CustomItemInput{Name: ""}); err == nil {
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
	svc, hub, tenantID := newSvc(t)
	ctx := tctx(tenantID)

	item, err := svc.Create(ctx, CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.BulkDelete(ctx, []string{item.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "custom_item" || e.UUID != "" || e.Action != "bulk_delete" {
			t.Fatalf("event=%+v want custom_item/0/bulk_delete", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after BulkDelete")
	}
}

func TestCustomItemListSearchGet(t *testing.T) {
	svc, _, tenantID := newSvc(t)
	ctx := tctx(tenantID)

	widget, err := svc.Create(ctx, CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create widget: %v", err)
	}
	if _, err := svc.Create(ctx, CustomItemInput{Name: "Gadget", Rate: 7}); err != nil {
		t.Fatalf("Create gadget: %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List = %d, want 2", len(list))
	}

	found, err := svc.Search(ctx, "Widget")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(found) != 1 || found[0].ID != widget.ID {
		t.Fatalf("Search Widget = %+v, want one id %s", found, widget.ID)
	}

	got, err := svc.Get(ctx, widget.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Name != "Widget" {
		t.Fatalf("Get = %+v, want Widget", got)
	}
}

func TestCustomItemGetNotFoundReturnsNil(t *testing.T) {
	svc, _, tenantID := newSvc(t)

	// A missing row now surfaces apperr.ErrNotFound from the service (the handler
	// 404s) instead of the old ambiguous (nil, nil).
	got, err := svc.Get(tctx(tenantID), "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c")
	if !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("Get missing: err = %v, want apperr.ErrNotFound", err)
	}
	if got != nil {
		t.Fatalf("Get missing = %+v, want nil", got)
	}
}

func TestCustomItemUpdateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newSvc(t)
	ctx := tctx(tenantID)

	item, err := svc.Create(ctx, CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	updated, err := svc.Update(ctx, item.ID, CustomItemInput{Name: "Widget Pro", Rate: 9})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil || updated.Name != "Widget Pro" {
		t.Fatalf("Update = %+v, want Widget Pro", updated)
	}
	select {
	case e := <-ch:
		if e.Entity != "custom_item" || e.UUID != item.ID || e.Action != "update" {
			t.Fatalf("event=%+v want custom_item/%s/update", e, item.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Update")
	}
}

func TestCustomItemUpdateNotFoundReturnsNil(t *testing.T) {
	svc, _, tenantID := newSvc(t)

	// Updating a missing row now surfaces apperr.ErrNotFound (the handler 404s)
	// instead of the old ambiguous (nil, nil).
	got, err := svc.Update(tctx(tenantID), "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c", CustomItemInput{Name: "X", Rate: 1})
	if !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("Update missing: err = %v, want apperr.ErrNotFound", err)
	}
	if got != nil {
		t.Fatalf("Update missing = %+v, want nil", got)
	}
}

func TestCustomItemDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID := newSvc(t)
	ctx := tctx(tenantID)

	item, err := svc.Create(ctx, CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.Delete(ctx, item.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "custom_item" || e.UUID != item.ID || e.Action != "delete" {
			t.Fatalf("event=%+v want custom_item/%s/delete", e, item.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Delete")
	}

	// After delete the row is gone, so Get now returns apperr.ErrNotFound.
	got, err := svc.Get(ctx, item.ID)
	if !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("Get after delete: err = %v, want apperr.ErrNotFound", err)
	}
	if got != nil {
		t.Fatalf("custom item %s still present after delete", item.ID)
	}
}

func TestCustomItemTenantScoping(t *testing.T) {
	conn := newTestDB(t)
	hub := realtime.NewHub()
	svc := NewService(conn, hub)

	tenantA := seedTenant(t, conn)
	tenantB := seedTenant(t, conn)

	item, err := svc.Create(tctx(tenantA), CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}

	listB, err := svc.List(tctx(tenantB))
	if err != nil {
		t.Fatalf("List B: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("tenant B sees %d custom items, want 0", len(listB))
	}

	// Tenant B's Get of tenant A's row is a miss, now apperr.ErrNotFound.
	gotB, err := svc.Get(tctx(tenantB), item.ID)
	if !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("Get B: err = %v, want apperr.ErrNotFound", err)
	}
	if gotB != nil {
		t.Fatalf("tenant B fetched tenant A custom item %s", item.ID)
	}
}
