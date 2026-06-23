package session

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/realtime"
)

// customItem is a pre-priced custom line (no catalogue code), so repo tests don't
// need catalogue/zone seeding — pricing is exercised separately by billing tests.
func customItem(desc string, qty, price float64) billing.LineItemInput {
	return billing.LineItemInput{Description: desc, Unit: "EA", Quantity: qty, UnitPrice: price}
}

func TestSessionItemCRUDAndUnbilledGuards(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedClient(t, conn, tid, "Jane")
	repo := NewSessions(conn)
	ctx := context.Background()

	sh, err := repo.Create(ctx, tid, nil, sampleSessionInput(pid))
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	it, err := repo.CreateItem(ctx, tid, sh.ID, customItem("travel", 36, 1.0))
	if err != nil || it == nil {
		t.Fatalf("CreateItem = %+v err=%v", it, err)
	}
	if it.SessionID == nil || *it.SessionID != sh.ID || it.InvoiceID != nil {
		t.Fatalf("new item must be session-scoped + unbilled: %+v", it)
	}
	if it.LineTotal != 36 {
		t.Fatalf("line total = %v, want 36", it.LineTotal)
	}

	items, err := repo.ListItems(ctx, tid, sh.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListItems = %d err=%v", len(items), err)
	}
	if n, _ := repo.CountItems(ctx, tid, sh.ID); n != 1 {
		t.Fatalf("CountItems = %d, want 1", n)
	}

	// Update the unbilled item.
	up, err := repo.UpdateItem(ctx, tid, it.ID, customItem("travel", 40, 1.0))
	if err != nil || up == nil || up.Quantity != 40 || up.LineTotal != 40 {
		t.Fatalf("UpdateItem = %+v err=%v", up, err)
	}

	// Link the item to an invoice → it becomes billed.
	invID := seedInvoice(t, conn, tid, pid, 100)
	if err := gen.New(conn).LinkSessionItemsToInvoice(ctx, gen.LinkSessionItemsToInvoiceParams{
		InvoiceID: sql.NullInt64{Int64: invID, Valid: true},
		SortOrder: sql.NullInt64{Int64: 0, Valid: true},
		TenantID:  tid, SessionID: sql.NullInt64{Int64: sh.ID, Valid: true},
	}); err != nil {
		t.Fatalf("link item: %v", err)
	}

	// Billed items are excluded from the unbilled count and are immutable here.
	if n, _ := repo.CountItems(ctx, tid, sh.ID); n != 0 {
		t.Fatalf("CountItems after billing = %d, want 0", n)
	}
	billedUpd, err := repo.UpdateItem(ctx, tid, it.ID, customItem("travel", 99, 1.0))
	if err != nil || billedUpd != nil {
		t.Fatalf("UpdateItem on billed item = %+v err=%v, want (nil,nil)", billedUpd, err)
	}
	if err := repo.DeleteItem(ctx, tid, it.ID); err != nil {
		t.Fatalf("DeleteItem (no-op on billed): %v", err)
	}
	if items, _ := repo.ListItems(ctx, tid, sh.ID); len(items) != 1 {
		t.Fatalf("billed item must survive DeleteItem guard, got %d", len(items))
	}
}

func TestSessionDeleteBilledGuard(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedClient(t, conn, tid, "Jane")
	svc := NewService(conn, conn, realtime.NewHub(), nil)
	repo := NewSessions(conn)
	ctx := tctx(tid)

	sh, err := repo.Create(ctx, tid, nil, sampleSessionInput(pid))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Recorded session deletes fine.
	other, _ := repo.Create(ctx, tid, nil, sampleSessionInput(pid))
	if err := svc.Delete(ctx, other.UUID); err != nil {
		t.Fatalf("Delete recorded session: %v", err)
	}

	// Draft it (status past 'recorded') → delete must be refused.
	invID := seedInvoice(t, conn, tid, pid, 100)
	if err := repo.SetInvoice(ctx, tid, sh.ID, invID, "drafted"); err != nil {
		t.Fatalf("SetInvoice: %v", err)
	}
	if err := svc.Delete(ctx, sh.UUID); !errors.Is(err, ErrSessionBilled) {
		t.Fatalf("Delete billed session err = %v, want ErrSessionBilled", err)
	}
	if got, _ := repo.Get(ctx, tid, sh.ID); got == nil {
		t.Fatal("billed session must survive a refused delete")
	}
}
