package client

import (
	"errors"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
)

func newClientSvc(t *testing.T) (*Service, *realtime.Hub, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	hub := realtime.NewHub()
	return NewService(conn, hub), hub, tenantID
}

func TestClientCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID := newClientSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	c, err := svc.Create(tctx(tenantID), ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c == nil {
		t.Fatal("Create returned nil client")
	}

	select {
	case e := <-ch:
		if e.Entity != "client" || e.UUID != c.UUID || e.Action != "create" {
			t.Fatalf("event=%+v want client/%d/create", e, c.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestClientCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub, tenantID := newClientSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), ClientInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

// TestClientTypeFieldGating exercises the Phase 6 type-driven field gating on
// create: a standard client needs only a name; an ndis client must carry plan
// dates + mgmt type; an invalid type is a clean validation error (not a DB 500).
func TestClientTypeFieldGating(t *testing.T) {
	svc, _, tenantID := newClientSvc(t)
	ctx := tctx(tenantID)

	// (a) standard client with only a name is accepted (type defaults to standard).
	c, err := svc.Create(ctx, ClientInput{Name: "Generic Co"})
	if err != nil {
		t.Fatalf("standard client with only a name must be accepted: %v", err)
	}
	if c == nil || c.Type != "standard" {
		t.Fatalf("want stored type=standard, got %+v", c)
	}

	// (b) ndis client missing plan dates is rejected with a field-level error.
	_, err = svc.Create(ctx, ClientInput{Name: "NDIS Co", Type: "ndis"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("ndis client missing plan dates: want *ValidationError, got %T: %v", err, err)
	}
	var sawStart, sawEnd bool
	for _, fe := range ve.Errors {
		if fe.Field == "planStart" {
			sawStart = true
		}
		if fe.Field == "planEnd" {
			sawEnd = true
		}
	}
	if !sawStart || !sawEnd {
		t.Fatalf("want planStart AND planEnd field errors, got %+v", ve.Errors)
	}

	// (b2) ndis client WITH the required fields is accepted.
	if _, err := svc.Create(ctx, ClientInput{
		Name: "NDIS Co", Type: "ndis",
		PlanStart: "2025-07-01", PlanEnd: "2026-06-30", MgmtType: "self",
	}); err != nil {
		t.Fatalf("complete ndis client must be accepted: %v", err)
	}

	// (c) invalid type is a clean validation error (not a DB CHECK 500).
	_, err = svc.Create(ctx, ClientInput{Name: "Bad", Type: "foo"})
	if !errors.Is(err, errInvalidType) {
		t.Fatalf("invalid type: want errInvalidType, got %T: %v", err, err)
	}
}

func TestClientBulkDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID := newClientSvc(t)
	ctx := tctx(tenantID)

	c, err := svc.Create(ctx, ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.BulkDelete(ctx, []int64{c.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "client" || e.UUID != "" || e.Action != "bulk_delete" {
			t.Fatalf("event=%+v want client/0/bulk_delete", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after BulkDelete")
	}
}
