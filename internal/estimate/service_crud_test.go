package estimate

import (
	"errors"
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
)

func TestEstimateListAndGet(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est := makeEstimate(t, svc, tenantID, clientID)

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != est.ID {
		t.Fatalf("List = %+v, want one id %s", list, est.ID)
	}

	got, err := svc.Get(ctx, est.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ID != est.ID {
		t.Fatalf("Get = %+v, want id %s", got, est.ID)
	}
}

func TestEstimateGetNotFoundReturnsNil(t *testing.T) {
	svc, tenantID, _ := newEstimateSvc(t)

	got, err := svc.Get(tctx(tenantID), "nonexistent-uuid")
	if err != nil {
		t.Fatalf("Get missing: unexpected err %v", err)
	}
	if got != nil {
		t.Fatalf("Get missing = %+v, want nil", got)
	}
}

func TestEstimateListByStatusAndClientSvc(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est := makeEstimate(t, svc, tenantID, clientID)
	if err := svc.UpdateStatus(ctx, est.ID, "accepted"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	accepted, err := svc.ListByStatus(ctx, "accepted")
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(accepted) != 1 || accepted[0].ID != est.ID {
		t.Fatalf("ListByStatus accepted = %+v, want one id %s", accepted, est.ID)
	}

	byPart, err := svc.ListClientEstimates(ctx, clientID)
	if err != nil {
		t.Fatalf("ListClientEstimates: %v", err)
	}
	if len(byPart) != 1 || byPart[0].ID != est.ID {
		t.Fatalf("ListClientEstimates = %+v, want one id %s", byPart, est.ID)
	}
}

func TestEstimateUpdateSvc(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est := makeEstimate(t, svc, tenantID, clientID)

	updated, err := svc.Update(ctx, est.ID, EstimateInput{
		ClientID: clientID, IssueDate: "2026-05-01", ValidUntil: "2026-06-01",
	}, []billing.LineItemInput{{Description: "B", Quantity: 2, UnitPrice: 8}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("Update returned nil")
	}
	if updated.IssueDate != "2026-05-01" {
		t.Fatalf("Update IssueDate = %q, want 2026-05-01", updated.IssueDate)
	}
}

func TestEstimateUpdateSvcNotFoundReturnsNil(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)

	got, err := svc.Update(tctx(tenantID), "nonexistent-uuid", EstimateInput{
		ClientID: clientID, IssueDate: "2026-05-01", ValidUntil: "2026-06-01",
	}, []billing.LineItemInput{{Description: "B", Quantity: 1, UnitPrice: 1}})
	if err != nil {
		t.Fatalf("Update missing: unexpected err %v", err)
	}
	if got != nil {
		t.Fatalf("Update missing = %+v, want nil", got)
	}
}

func TestEstimateDelete(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est := makeEstimate(t, svc, tenantID, clientID)

	if err := svc.Delete(ctx, est.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := svc.Get(ctx, est.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("estimate %s still present after delete", est.ID)
	}
}

func TestEstimateBulkDelete(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	a := makeEstimate(t, svc, tenantID, clientID)
	b := makeEstimate(t, svc, tenantID, clientID)

	if err := svc.BulkDelete(ctx, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List after bulk delete = %d, want 0", len(list))
	}
}

func TestEstimateBulkUpdateStatus(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	a := makeEstimate(t, svc, tenantID, clientID)
	b := makeEstimate(t, svc, tenantID, clientID)

	if err := svc.BulkUpdateStatus(ctx, []string{a.ID, b.ID}, "sent"); err != nil {
		t.Fatalf("BulkUpdateStatus: %v", err)
	}

	sent, err := svc.ListByStatus(ctx, "sent")
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("sent estimates = %d, want 2", len(sent))
	}
}

// TestEstimateConvertNotAccepted asserts converting a draft estimate propagates
// ErrNotAccepted unchanged (no invoice created).
func TestEstimateConvertNotAccepted(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est := makeEstimate(t, svc, tenantID, clientID) // status defaults to draft

	res, err := svc.Convert(ctx, est.ID)
	if !errors.Is(err, ErrNotAccepted) {
		t.Fatalf("Convert draft err = %v, want ErrNotAccepted", err)
	}
	if res != nil {
		t.Fatalf("Convert draft res = %+v, want nil", res)
	}
}

// TestEstimateConvertAlreadyConverted asserts a second convert propagates
// ErrAlreadyConverted.
func TestEstimateConvertAlreadyConverted(t *testing.T) {
	svc, tenantID, clientID := newEstimateSvc(t)
	ctx := tctx(tenantID)

	est := makeEstimate(t, svc, tenantID, clientID)
	if err := svc.UpdateStatus(ctx, est.ID, "accepted"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if _, err := svc.Convert(ctx, est.ID); err != nil {
		t.Fatalf("first Convert: %v", err)
	}

	res, err := svc.Convert(ctx, est.ID)
	if !errors.Is(err, ErrAlreadyConverted) {
		t.Fatalf("second Convert err = %v, want ErrAlreadyConverted", err)
	}
	if res != nil {
		t.Fatalf("second Convert res = %+v, want nil", res)
	}
}

// TestEstimateTenantScoping asserts cross-tenant isolation on List/Get.
func TestEstimateTenantScoping(t *testing.T) {
	conn := newTestDB(t)
	svc := NewService(conn)

	tenantA := seedTenant(t, conn, "Acme")
	partA := seedClient(t, conn, tenantA, "Jane")
	tenantB := seedTenant(t, conn, "Beta")

	est := makeEstimate(t, svc, tenantA, partA)

	listB, err := svc.List(tctx(tenantB))
	if err != nil {
		t.Fatalf("List B: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("tenant B sees %d estimates, want 0", len(listB))
	}

	gotB, err := svc.Get(tctx(tenantB), est.ID)
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}
	if gotB != nil {
		t.Fatalf("tenant B fetched tenant A estimate %s", est.ID)
	}
}
