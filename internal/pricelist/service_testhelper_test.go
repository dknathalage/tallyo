package pricelist

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/google/uuid"
)

func seedTenant(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		Uuid:      uuid.NewString(),
		Name:      "Acme NDIS",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedTenant: %v", err)
	}
	return tn.ID
}

func tctx(tenantID int64) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}

func seedZonedCatalog(t *testing.T, conn *sql.DB, label, from, to, code string, gstFree bool, prices map[string]*float64) int64 {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	now := time.Now().UTC().Format(time.RFC3339)
	var et sql.NullString
	if to != "" {
		et = sql.NullString{String: to, Valid: true}
	}
	v, err := q.CreatePriceListVersion(ctx, gen.CreatePriceListVersionParams{
		Uuid: uuid.NewString(), Label: label, EffectiveFrom: from, EffectiveTo: et, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreatePriceListVersion: %v", err)
	}
	tx := int64(1) // taxable is the inverse of gst-free
	if gstFree {
		tx = 0
	}
	si, err := q.CreateItem(ctx, gen.CreateItemParams{
		Uuid: uuid.NewString(), PriceListVersionID: v.ID, Code: code, Name: "Item " + code, Taxable: tx,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	for zone, capPtr := range prices { // bounded by len(prices)
		var pc sql.NullFloat64
		if capPtr != nil {
			pc = sql.NullFloat64{Float64: *capPtr, Valid: true}
		}
		if _, err := q.CreateItemPrice(ctx, gen.CreateItemPriceParams{
			ItemID: si.ID, Zone: zone, PriceCap: pc,
		}); err != nil {
			t.Fatalf("CreateItemPrice %s: %v", zone, err)
		}
	}
	return v.ID
}

func addItemToVersion(t *testing.T, conn *sql.DB, versionID int64, code string, gstFree bool, prices map[string]*float64) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	tx := int64(1) // taxable is the inverse of gst-free
	if gstFree {
		tx = 0
	}
	si, err := q.CreateItem(ctx, gen.CreateItemParams{
		Uuid: uuid.NewString(), PriceListVersionID: versionID, Code: code, Name: "Item " + code, Taxable: tx,
	})
	if err != nil {
		t.Fatalf("CreateItem %s: %v", code, err)
	}
	for zone, capPtr := range prices { // bounded by len(prices)
		var pc sql.NullFloat64
		if capPtr != nil {
			pc = sql.NullFloat64{Float64: *capPtr, Valid: true}
		}
		if _, err := q.CreateItemPrice(ctx, gen.CreateItemPriceParams{
			ItemID: si.ID, Zone: zone, PriceCap: pc,
		}); err != nil {
			t.Fatalf("CreateItemPrice %s/%s: %v", code, zone, err)
		}
	}
}

func fptr(f float64) *float64 { return &f }
