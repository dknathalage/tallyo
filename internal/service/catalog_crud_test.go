package service

import (
	"context"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func TestCustomItemListSearchGet(t *testing.T) {
	svc, _, tenantID := newCustomItemSvc(t)
	ctx := tctx(tenantID)

	widget, err := svc.Create(ctx, repository.CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create widget: %v", err)
	}
	if _, err := svc.Create(ctx, repository.CustomItemInput{Name: "Gadget", Rate: 7}); err != nil {
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
		t.Fatalf("Search Widget = %+v, want one id %d", found, widget.ID)
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
	svc, _, tenantID := newCustomItemSvc(t)

	got, err := svc.Get(tctx(tenantID), 999999)
	if err != nil {
		t.Fatalf("Get missing: unexpected err %v", err)
	}
	if got != nil {
		t.Fatalf("Get missing = %+v, want nil", got)
	}
}

func TestCustomItemUpdateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newCustomItemSvc(t)
	ctx := tctx(tenantID)

	item, err := svc.Create(ctx, repository.CustomItemInput{Name: "Widget", Rate: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	updated, err := svc.Update(ctx, item.ID, repository.CustomItemInput{Name: "Widget Pro", Rate: 9})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil || updated.Name != "Widget Pro" {
		t.Fatalf("Update = %+v, want Widget Pro", updated)
	}
	select {
	case e := <-ch:
		if e.Entity != "custom_item" || e.ID != item.ID || e.Action != "update" {
			t.Fatalf("event=%+v want custom_item/%d/update", e, item.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Update")
	}
}

func TestCustomItemUpdateNotFoundReturnsNil(t *testing.T) {
	svc, _, tenantID := newCustomItemSvc(t)

	got, err := svc.Update(tctx(tenantID), 999999, repository.CustomItemInput{Name: "X", Rate: 1})
	if err != nil {
		t.Fatalf("Update missing: unexpected err %v", err)
	}
	if got != nil {
		t.Fatalf("Update missing = %+v, want nil", got)
	}
}

func TestCustomItemDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID := newCustomItemSvc(t)
	ctx := tctx(tenantID)

	item, err := svc.Create(ctx, repository.CustomItemInput{Name: "Widget", Rate: 5})
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
		if e.Entity != "custom_item" || e.ID != item.ID || e.Action != "delete" {
			t.Fatalf("event=%+v want custom_item/%d/delete", e, item.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Delete")
	}

	got, err := svc.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("custom item %d still present after delete", item.ID)
	}
}

// TestCustomItemTenantScoping asserts a custom item is invisible to other tenants.
func TestCustomItemTenantScoping(t *testing.T) {
	conn := newTestDB(t)
	hub := realtime.NewHub()
	svc := NewCustomItemService(conn, hub)

	tenantA := seedTenant(t, conn)
	tenantB := seedTenant(t, conn)

	item, err := svc.Create(tctx(tenantA), repository.CustomItemInput{Name: "Widget", Rate: 5})
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

	gotB, err := svc.Get(tctx(tenantB), item.ID)
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}
	if gotB != nil {
		t.Fatalf("tenant B fetched tenant A custom item %d", item.ID)
	}
}

// TestSupportCatalogGetVersionAndListPrices ingests a small catalogue then
// exercises the read-only GetVersion + ListPrices methods.
func TestSupportCatalogGetVersionAndListPrices(t *testing.T) {
	conn := newTestDB(t)
	hub := realtime.NewHub()
	ingest := NewCatalogIngestService(conn, hub)
	read := NewSupportCatalogService(conn)
	ctx := context.Background()

	data := catalogXLSX(t, catalogHeaders, [][]string{
		{"01_011_0107_1_1", "Assistance With Self-Care", "Hour", "Core", "Daily Living", "$67.56", "$94.58", "$101.34"},
	})
	summary, err := ingest.IngestXLSX(ctx, data, "v1", "2025-07-01", "c.xlsx")
	if err != nil {
		t.Fatalf("IngestXLSX: %v", err)
	}

	ver, err := read.GetVersion(ctx, summary.VersionID)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if ver == nil || ver.ID != summary.VersionID {
		t.Fatalf("GetVersion = %+v, want id %d", ver, summary.VersionID)
	}

	items, err := read.ListSupportItems(ctx, summary.VersionID)
	if err != nil {
		t.Fatalf("ListSupportItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}

	prices, err := read.ListPrices(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("ListPrices: %v", err)
	}
	if len(prices) != 3 {
		t.Fatalf("ListPrices = %d, want 3 zone rows", len(prices))
	}
}

// TestSupportCatalogGetVersionMissingReturnsNil asserts an absent version id
// yields (nil, nil) rather than an error.
func TestSupportCatalogGetVersionMissingReturnsNil(t *testing.T) {
	conn := newTestDB(t)
	read := NewSupportCatalogService(conn)

	ver, err := read.GetVersion(context.Background(), 999999)
	if err != nil {
		t.Fatalf("GetVersion missing: unexpected err %v", err)
	}
	if ver != nil {
		t.Fatalf("GetVersion missing = %+v, want nil", ver)
	}
}
