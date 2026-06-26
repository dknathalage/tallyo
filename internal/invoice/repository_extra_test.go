package invoice

import (
	"context"
	"testing"
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
		t.Fatalf("sent = %+v, want one (id=%s)", sent, a.ID)
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
			t.Fatalf("invoice %d client = %s, want %s", i, janeInvs[i].ClientID, jane)
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
	if err := repo.BulkUpdateStatus(ctx, tid, []string{a.ID, b.ID}, "sent"); err != nil {
		t.Fatalf("BulkUpdateStatus: %v", err)
	}
	sent, _ := repo.ListByStatus(ctx, tid, "sent")
	if len(sent) != 2 {
		t.Fatalf("sent after bulk = %d, want 2", len(sent))
	}

	// Delete a+b in bulk; c remains.
	if err := repo.BulkDelete(ctx, tid, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	list, _ := repo.List(ctx, tid)
	if len(list) != 1 || list[0].ID != c.ID {
		t.Fatalf("after bulk delete list = %+v, want only c (id=%s)", list, c.ID)
	}
}
