package estimate

import (
	"context"
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
)

func TestEstimateUpdate(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedClient(t, conn, tid, "Jane")
	repo := NewEstimates(conn)
	ctx := context.Background()

	est := mkEstimate(t, repo, tid, pid)

	// Missing client and empty items are rejected.
	if _, err := repo.Update(ctx, tid, est.ID, EstimateInput{ClientID: 0}, []billing.LineItemInput{{Description: "X", Quantity: 1, UnitPrice: 1}}); err == nil {
		t.Fatal("Update with no client: want error")
	}
	if _, err := repo.Update(ctx, tid, est.ID, EstimateInput{ClientID: pid}, nil); err == nil {
		t.Fatal("Update with no items: want error")
	}

	up, err := repo.Update(ctx, tid, est.ID, EstimateInput{
		ClientID: pid, IssueDate: "2026-01-01", ValidUntil: "2026-03-01", Tax: 5,
	}, []billing.LineItemInput{{Description: "Y", Quantity: 3, UnitPrice: 10}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Number immutable; totals recomputed (30 + 5 = 35); one item.
	if up.Number != est.Number || up.Subtotal != 30 || up.Total != 35 || len(up.LineItems) != 1 {
		t.Fatalf("Update = %+v", up)
	}

	// Updating a non-existent estimate returns (nil, nil).
	missing, err := repo.Update(ctx, tid, 999999, EstimateInput{ClientID: pid},
		[]billing.LineItemInput{{Description: "Z", Quantity: 1, UnitPrice: 1}})
	if err != nil || missing != nil {
		t.Fatalf("Update missing = %+v err=%v, want nil/nil", missing, err)
	}
}

func TestEstimateListByStatusAndClient(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	jane := seedClient(t, conn, tid, "Jane")
	bob := seedClient(t, conn, tid, "Bob")
	repo := NewEstimates(conn)
	ctx := context.Background()

	a := mkEstimate(t, repo, tid, jane)
	mkEstimate(t, repo, tid, bob)
	if err := repo.UpdateStatus(ctx, tid, a.ID, "sent"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	sent, err := repo.ListByStatus(ctx, tid, "sent")
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(sent) != 1 || sent[0].ID != a.ID {
		t.Fatalf("sent = %+v, want only a (id=%d)", sent, a.ID)
	}

	janeEsts, err := repo.ListClientEstimates(ctx, tid, jane)
	if err != nil {
		t.Fatalf("ListClientEstimates: %v", err)
	}
	if len(janeEsts) != 1 || janeEsts[0].ClientID == nil || *janeEsts[0].ClientID != jane {
		t.Fatalf("jane estimates = %+v, want one for jane", janeEsts)
	}
}

func TestEstimateBulkDeleteAndBulkStatus(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedClient(t, conn, tid, "Jane")
	repo := NewEstimates(conn)
	ctx := context.Background()

	a := mkEstimate(t, repo, tid, pid)
	b := mkEstimate(t, repo, tid, pid)
	c := mkEstimate(t, repo, tid, pid)

	// Empty slices are no-ops.
	if err := repo.BulkDelete(ctx, tid, nil); err != nil {
		t.Fatalf("BulkDelete empty: %v", err)
	}
	if err := repo.BulkUpdateStatus(ctx, tid, nil, "sent"); err != nil {
		t.Fatalf("BulkUpdateStatus empty: %v", err)
	}

	if err := repo.BulkUpdateStatus(ctx, tid, []int64{a.ID, b.ID}, "sent"); err != nil {
		t.Fatalf("BulkUpdateStatus: %v", err)
	}
	if sent, _ := repo.ListByStatus(ctx, tid, "sent"); len(sent) != 2 {
		t.Fatalf("sent after bulk = %d, want 2", len(sent))
	}

	if err := repo.BulkDelete(ctx, tid, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	list, _ := repo.List(ctx, tid)
	if len(list) != 1 || list[0].ID != c.ID {
		t.Fatalf("after bulk delete = %+v, want only c (id=%d)", list, c.ID)
	}
}
