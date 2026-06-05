package repository

import (
	"context"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

// clientFixture spins up a migrated DB plus a clients repo, and seeds one rate
// tier and one payer so join names can be asserted.
type clientFixture struct {
	repo    *ClientsRepo
	tierID  int64
	tierNm  string
	payerID int64
	payerNm string
}

func newClientFixture(t *testing.T) clientFixture {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "client.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	tier, err := NewRateTiers(conn).Create(ctx, RateTierInput{Name: "Gold", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create tier: %v", err)
	}
	payer, err := NewPayers(conn).Create(ctx, PayerInput{Name: "BigCo"})
	if err != nil {
		t.Fatalf("Create payer: %v", err)
	}
	return clientFixture{
		repo:    NewClients(conn),
		tierID:  tier.ID,
		tierNm:  tier.Name,
		payerID: payer.ID,
		payerNm: payer.Name,
	}
}

func TestClientCreate(t *testing.T) {
	f := newClientFixture(t)
	ctx := context.Background()

	c, err := f.repo.Create(ctx, ClientInput{
		Name:          "Acme",
		Email:         "a@b.com",
		PricingTierID: &f.tierID,
		PayerID:       &f.payerID,
		Metadata:      "",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c == nil || c.ID <= 0 {
		t.Fatalf("Create = %+v, want ID > 0", c)
	}
	if c.Name != "Acme" || c.Email != "a@b.com" {
		t.Fatalf("Create = %+v, want Acme/a@b.com", c)
	}
	if c.Metadata != "{}" {
		t.Fatalf("Metadata = %q, want {}", c.Metadata)
	}
	if c.PricingTierID == nil || *c.PricingTierID != f.tierID {
		t.Fatalf("PricingTierID = %v, want %d", c.PricingTierID, f.tierID)
	}
	if c.PricingTierName != f.tierNm {
		t.Fatalf("PricingTierName = %q, want %q", c.PricingTierName, f.tierNm)
	}
	if c.PayerID == nil || *c.PayerID != f.payerID {
		t.Fatalf("PayerID = %v, want %d", c.PayerID, f.payerID)
	}
	if c.PayerName != f.payerNm {
		t.Fatalf("PayerName = %q, want %q", c.PayerName, f.payerNm)
	}
}

func TestClientCreateNilFKs(t *testing.T) {
	f := newClientFixture(t)
	ctx := context.Background()

	c, err := f.repo.Create(ctx, ClientInput{Name: "NoFK"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.PricingTierID != nil {
		t.Fatalf("PricingTierID = %v, want nil", c.PricingTierID)
	}
	if c.PricingTierName != "" {
		t.Fatalf("PricingTierName = %q, want empty", c.PricingTierName)
	}
	if c.PayerID != nil {
		t.Fatalf("PayerID = %v, want nil", c.PayerID)
	}
	if c.PayerName != "" {
		t.Fatalf("PayerName = %q, want empty", c.PayerName)
	}
}

func TestClientCreateRejectsEmptyName(t *testing.T) {
	f := newClientFixture(t)
	if _, err := f.repo.Create(context.Background(), ClientInput{Name: ""}); err == nil {
		t.Fatal("Create with empty name: want error, got nil")
	}
}

func TestClientListJoinNames(t *testing.T) {
	f := newClientFixture(t)
	ctx := context.Background()

	withFK, err := f.repo.Create(ctx, ClientInput{
		Name:          "Acme",
		Email:         "acme@b.com",
		PricingTierID: &f.tierID,
		PayerID:       &f.payerID,
	})
	if err != nil {
		t.Fatalf("Create withFK: %v", err)
	}
	if _, err := f.repo.Create(ctx, ClientInput{Name: "Zeta"}); err != nil {
		t.Fatalf("Create Zeta: %v", err)
	}

	list, err := f.repo.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list == nil {
		t.Fatal("List returned nil slice")
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}

	var fkRow, nilRow *Client
	for _, c := range list {
		switch c.ID {
		case withFK.ID:
			fkRow = c
		default:
			nilRow = c
		}
	}
	if fkRow == nil || nilRow == nil {
		t.Fatalf("expected both rows; fk=%v nil=%v", fkRow, nilRow)
	}
	if fkRow.PricingTierName != f.tierNm || fkRow.PayerName != f.payerNm {
		t.Fatalf("fkRow names = %q/%q, want %q/%q", fkRow.PricingTierName, fkRow.PayerName, f.tierNm, f.payerNm)
	}
	if fkRow.PricingTierID == nil || *fkRow.PricingTierID != f.tierID {
		t.Fatalf("fkRow.PricingTierID = %v, want %d", fkRow.PricingTierID, f.tierID)
	}
	if fkRow.PayerID == nil || *fkRow.PayerID != f.payerID {
		t.Fatalf("fkRow.PayerID = %v, want %d", fkRow.PayerID, f.payerID)
	}
	if nilRow.PricingTierName != "" || nilRow.PricingTierID != nil {
		t.Fatalf("nilRow tier = %q/%v, want empty/nil", nilRow.PricingTierName, nilRow.PricingTierID)
	}
}

func TestClientListSearch(t *testing.T) {
	f := newClientFixture(t)
	ctx := context.Background()

	if _, err := f.repo.Create(ctx, ClientInput{Name: "Acme", Email: "acme@b.com"}); err != nil {
		t.Fatalf("Create Acme: %v", err)
	}
	if _, err := f.repo.Create(ctx, ClientInput{Name: "Zeta", Email: "zeta@b.com"}); err != nil {
		t.Fatalf("Create Zeta: %v", err)
	}

	byName, err := f.repo.List(ctx, "acm")
	if err != nil {
		t.Fatalf("List acm: %v", err)
	}
	if len(byName) != 1 || byName[0].Name != "Acme" {
		t.Fatalf("List acm = %+v, want [Acme]", byName)
	}

	byEmail, err := f.repo.List(ctx, "zeta@")
	if err != nil {
		t.Fatalf("List zeta@: %v", err)
	}
	if len(byEmail) != 1 || byEmail[0].Name != "Zeta" {
		t.Fatalf("List zeta@ = %+v, want [Zeta]", byEmail)
	}
}

func TestClientGet(t *testing.T) {
	f := newClientFixture(t)
	ctx := context.Background()

	created, err := f.repo.Create(ctx, ClientInput{
		Name:          "Acme",
		PricingTierID: &f.tierID,
		PayerID:       &f.payerID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := f.repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Name != "Acme" {
		t.Fatalf("Get = %+v, want Name=Acme", got)
	}
	if got.PricingTierName != f.tierNm || got.PayerName != f.payerNm {
		t.Fatalf("Get names = %q/%q, want %q/%q", got.PricingTierName, got.PayerName, f.tierNm, f.payerNm)
	}

	missing, err := f.repo.Get(ctx, 99999)
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Get missing = %+v, want nil", missing)
	}
}

func TestClientUpdate(t *testing.T) {
	f := newClientFixture(t)
	ctx := context.Background()

	created, err := f.repo.Create(ctx, ClientInput{
		Name:          "Acme",
		PricingTierID: &f.tierID,
		PayerID:       &f.payerID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Change name + clear both FKs.
	updated, err := f.repo.Update(ctx, created.ID, ClientInput{Name: "Acme2"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil || updated.Name != "Acme2" {
		t.Fatalf("Update = %+v, want Name=Acme2", updated)
	}
	if updated.PricingTierID != nil || updated.PricingTierName != "" {
		t.Fatalf("Update tier = %v/%q, want nil/empty", updated.PricingTierID, updated.PricingTierName)
	}
	if updated.PayerID != nil || updated.PayerName != "" {
		t.Fatalf("Update payer = %v/%q, want nil/empty", updated.PayerID, updated.PayerName)
	}

	// Set FKs back.
	reset, err := f.repo.Update(ctx, created.ID, ClientInput{
		Name:          "Acme3",
		PricingTierID: &f.tierID,
		PayerID:       &f.payerID,
	})
	if err != nil {
		t.Fatalf("Update reset: %v", err)
	}
	if reset.PricingTierID == nil || *reset.PricingTierID != f.tierID {
		t.Fatalf("reset tier = %v, want %d", reset.PricingTierID, f.tierID)
	}
	if reset.PricingTierName != f.tierNm {
		t.Fatalf("reset tier name = %q, want %q", reset.PricingTierName, f.tierNm)
	}

	missing, err := f.repo.Update(ctx, 99999, ClientInput{Name: "Nope"})
	if err != nil {
		t.Fatalf("Update missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Update missing = %+v, want nil", missing)
	}
}

func TestClientDelete(t *testing.T) {
	f := newClientFixture(t)
	ctx := context.Background()

	c, err := f.repo.Create(ctx, ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := f.repo.Delete(ctx, c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := f.repo.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("row still present after delete: %+v", got)
	}
}

func TestClientBulkDelete(t *testing.T) {
	f := newClientFixture(t)
	ctx := context.Background()

	a, err := f.repo.Create(ctx, ClientInput{Name: "A"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	b, err := f.repo.Create(ctx, ClientInput{Name: "B"})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}

	if err := f.repo.BulkDelete(ctx, nil); err != nil {
		t.Fatalf("BulkDelete empty: %v", err)
	}
	if err := f.repo.BulkDelete(ctx, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	list, err := f.repo.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List len = %d, want 0 after bulk delete", len(list))
	}
}

func TestClientAuditRows(t *testing.T) {
	f := newClientFixture(t)
	ctx := context.Background()

	a, err := f.repo.Create(ctx, ClientInput{Name: "A"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	b, err := f.repo.Create(ctx, ClientInput{Name: "B"})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if _, err := f.repo.Update(ctx, a.ID, ClientInput{Name: "A2"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := f.repo.Delete(ctx, a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := f.repo.BulkDelete(ctx, []int64{b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}

	var n int
	if err := f.repo.db.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='client'",
	).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	// create A + create B + update A + delete A + bulk_delete = 5.
	if n != 5 {
		t.Fatalf("audit rows = %d, want 5", n)
	}

	// Create audit must carry the real (non-zero) entity id.
	var entID int64
	if err := f.repo.db.QueryRow(
		"SELECT entity_id FROM audit_log WHERE entity_type='client' AND action='create' ORDER BY id LIMIT 1",
	).Scan(&entID); err != nil {
		t.Fatalf("select create audit: %v", err)
	}
	if entID != a.ID {
		t.Fatalf("create audit entity_id = %d, want %d", entID, a.ID)
	}
}
