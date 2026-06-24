package pricelist

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

func seedTenant(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		Uuid:      ids.New(),
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

func tctx(tenantID int64) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}

func seedUnitPricedItem(t *testing.T, conn *sql.DB, tenantID int64, label, from, to, code string, gstFree bool, unitPrice float64) int64 {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	now := time.Now().UTC().Format(time.RFC3339)
	var et sql.NullString
	if to != "" {
		et = sql.NullString{String: to, Valid: true}
	}
	v, err := q.CreatePriceListVersion(ctx, gen.CreatePriceListVersionParams{
		TenantID: tenantID, Uuid: ids.New(), Label: label, EffectiveFrom: from, EffectiveTo: et, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreatePriceListVersion: %v", err)
	}
	tx := int64(1) // taxable is the inverse of gst-free
	if gstFree {
		tx = 0
	}
	if _, err := q.CreateItem(ctx, gen.CreateItemParams{
		TenantID: tenantID, Uuid: ids.New(), PriceListVersionID: v.ID, Code: code, Name: "Item " + code, Taxable: tx,
		UnitPrice: sql.NullFloat64{Float64: unitPrice, Valid: true},
	}); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	return v.ID
}

func addUnitPricedItemToVersion(t *testing.T, conn *sql.DB, tenantID, versionID int64, code string, gstFree bool, unitPrice float64) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	tx := int64(1) // taxable is the inverse of gst-free
	if gstFree {
		tx = 0
	}
	if _, err := q.CreateItem(ctx, gen.CreateItemParams{
		TenantID: tenantID, Uuid: ids.New(), PriceListVersionID: versionID, Code: code, Name: "Item " + code, Taxable: tx,
		UnitPrice: sql.NullFloat64{Float64: unitPrice, Valid: true},
	}); err != nil {
		t.Fatalf("CreateItem %s: %v", code, err)
	}
}
