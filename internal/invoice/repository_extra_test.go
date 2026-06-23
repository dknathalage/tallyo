package invoice

import (
	"context"
	"testing"
	"time"
)

func TestInvoiceListByStatus(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedClient(t, conn, tid, "Jane")
	repo := NewInvoices(conn)
	ctx := context.Background()

	a := mkInvoiceRepo(t, repo, tid, pid, "2026-01-31")
	mkInvoiceRepo(t, repo, tid, pid, "2026-02-28") // stays draft
	if err := repo.UpdateStatus(ctx, tid, a.ID, "sent"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	sent, err := repo.ListByStatus(ctx, tid, "sent")
	if err != nil {
		t.Fatalf("ListByStatus sent: %v", err)
	}
	if len(sent) != 1 || sent[0].ID != a.ID {
		t.Fatalf("sent = %+v, want one (id=%d)", sent, a.ID)
	}
	draft, err := repo.ListByStatus(ctx, tid, "draft")
	if err != nil {
		t.Fatalf("ListByStatus draft: %v", err)
	}
	if len(draft) != 1 {
		t.Fatalf("draft len = %d, want 1", len(draft))
	}
}

func TestInvoiceListClientInvoices(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	jane := seedClient(t, conn, tid, "Jane")
	bob := seedClient(t, conn, tid, "Bob")
	repo := NewInvoices(conn)
	ctx := context.Background()

	mkInvoiceRepo(t, repo, tid, jane, "2026-01-31")
	mkInvoiceRepo(t, repo, tid, jane, "2026-02-28")
	mkInvoiceRepo(t, repo, tid, bob, "2026-03-31")

	janeInvs, err := repo.ListClientInvoices(ctx, tid, jane)
	if err != nil {
		t.Fatalf("ListClientInvoices: %v", err)
	}
	if len(janeInvs) != 2 {
		t.Fatalf("jane invoices = %d, want 2", len(janeInvs))
	}
	for i := range janeInvs {
		if janeInvs[i].ClientID != jane {
			t.Fatalf("invoice %d client = %d, want %d", i, janeInvs[i].ClientID, jane)
		}
	}
}

func TestInvoiceBulkDeleteAndBulkStatus(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedClient(t, conn, tid, "Jane")
	repo := NewInvoices(conn)
	ctx := context.Background()

	a := mkInvoiceRepo(t, repo, tid, pid, "2026-01-31")
	b := mkInvoiceRepo(t, repo, tid, pid, "2026-02-28")
	c := mkInvoiceRepo(t, repo, tid, pid, "2026-03-31")

	// Empty slice is a no-op (no error, nothing deleted).
	if err := repo.BulkDelete(ctx, tid, nil); err != nil {
		t.Fatalf("BulkDelete empty: %v", err)
	}
	if err := repo.BulkUpdateStatus(ctx, tid, nil, "sent"); err != nil {
		t.Fatalf("BulkUpdateStatus empty: %v", err)
	}

	// Flip a+b to sent in bulk.
	if err := repo.BulkUpdateStatus(ctx, tid, []int64{a.ID, b.ID}, "sent"); err != nil {
		t.Fatalf("BulkUpdateStatus: %v", err)
	}
	sent, _ := repo.ListByStatus(ctx, tid, "sent")
	if len(sent) != 2 {
		t.Fatalf("sent after bulk = %d, want 2", len(sent))
	}

	// Delete a+b in bulk; c remains.
	if err := repo.BulkDelete(ctx, tid, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	list, _ := repo.List(ctx, tid)
	if len(list) != 1 || list[0].ID != c.ID {
		t.Fatalf("after bulk delete list = %+v, want only c (id=%d)", list, c.ID)
	}
}

func TestInvoiceMarkOverdueForTenant(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedClient(t, conn, tid, "Jane")
	repo := NewInvoices(conn)
	ctx := context.Background()

	past := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")
	future := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")

	overdueInv := mkInvoiceRepo(t, repo, tid, pid, past)
	notDue := mkInvoiceRepo(t, repo, tid, pid, future)
	draftPast := mkInvoiceRepo(t, repo, tid, pid, past)

	// Only 'sent' invoices are eligible; mark both past-due ones sent except draftPast.
	if err := repo.UpdateStatus(ctx, tid, overdueInv.ID, "sent"); err != nil {
		t.Fatalf("UpdateStatus overdueInv: %v", err)
	}
	if err := repo.UpdateStatus(ctx, tid, notDue.ID, "sent"); err != nil {
		t.Fatalf("UpdateStatus notDue: %v", err)
	}

	flipped, err := repo.MarkOverdueForTenant(ctx, tid)
	if err != nil {
		t.Fatalf("MarkOverdueForTenant: %v", err)
	}
	if len(flipped) != 1 || flipped[0].ID != overdueInv.ID {
		t.Fatalf("flipped = %+v, want only overdueInv (id=%d)", flipped, overdueInv.ID)
	}

	got, _ := repo.Get(ctx, tid, overdueInv.ID)
	if got.Status != "overdue" {
		t.Fatalf("overdueInv status = %q, want overdue", got.Status)
	}
	// draftPast stays draft (not sent), notDue stays sent.
	if g, _ := repo.Get(ctx, tid, draftPast.ID); g.Status != "draft" {
		t.Fatalf("draftPast status = %q, want draft", g.Status)
	}
	if g, _ := repo.Get(ctx, tid, notDue.ID); g.Status != "sent" {
		t.Fatalf("notDue status = %q, want sent", g.Status)
	}
}

func TestInvoiceMarkOverdueRequiresTenant(t *testing.T) {
	conn := newTestDB(t)
	repo := NewInvoices(conn)
	if _, err := repo.MarkOverdueForTenant(context.Background(), 0); err == nil {
		t.Fatal("MarkOverdueForTenant(0): want error")
	}
}

func TestInvoiceActiveTenantIDs(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "Active A")
	b := seedTenant(t, conn, "Active B")
	repo := NewInvoices(conn)

	ids, err := repo.ActiveTenantIDs(context.Background())
	if err != nil {
		t.Fatalf("ActiveTenantIDs: %v", err)
	}
	// Both seeded tenants are 'active'.
	seen := map[int64]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen[a] || !seen[b] {
		t.Fatalf("ActiveTenantIDs = %v, want to include %d and %d", ids, a, b)
	}
}
