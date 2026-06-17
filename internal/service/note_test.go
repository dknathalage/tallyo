package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/google/uuid"
)

func newNoteSvc(t *testing.T) (*NoteService, *realtime.Hub, int64, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	hub := realtime.NewHub()
	return NewNoteService(conn, hub), hub, tenantID, participantID
}

// seedNoteUser inserts a user so a note's author_user_id FK is satisfiable.
func seedNoteUser(t *testing.T, conn *sql.DB, tenantID int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	u, err := gen.New(conn).CreateUser(context.Background(), gen.CreateUserParams{
		Uuid: uuid.NewString(), TenantID: tenantID, Email: uuid.NewString() + "@x.com",
		PasswordHash: "x", Name: "U", Role: "member", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedNoteUser: %v", err)
	}
	return u.ID
}

// seedNoteInvoice inserts an invoice so a note's billed_invoice_id FK is
// satisfiable. Returns its id.
func seedNoteInvoice(t *testing.T, conn *sql.DB, tenantID, participantID int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	inv, err := gen.New(conn).CreateInvoice(context.Background(), gen.CreateInvoiceParams{
		Uuid: uuid.NewString(), TenantID: tenantID, Number: uuid.NewString(), ParticipantID: participantID,
		Status: "draft", IssueDate: "2026-01-01", DueDate: "2026-02-01", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedNoteInvoice: %v", err)
	}
	return inv.ID
}

func TestNoteCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantID := newNoteSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	n, err := svc.Create(ctx, repository.NoteInput{
		ParticipantID: participantID, ServiceDate: "2026-01-15", Body: "Visit",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n == nil {
		t.Fatal("Create returned nil note")
	}
	select {
	case e := <-ch:
		if e.Entity != "note" || e.ID != n.ID || e.Action != "create" {
			t.Fatalf("event=%+v want note/%d/create", e, n.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestNoteCreateAttributesAuthor(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	uid := seedNoteUser(t, conn, tenantID)
	svc := NewNoteService(conn, realtime.NewHub())
	ctx := reqctx.WithUser(tctx(tenantID), uid)

	n, err := svc.Create(ctx, repository.NoteInput{
		ParticipantID: participantID, ServiceDate: "2026-01-15", Body: "Visit",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.AuthorUserID == nil || *n.AuthorUserID != uid {
		t.Fatalf("author not attributed: %+v want %d", n.AuthorUserID, uid)
	}
}

func TestNoteListParticipantRange(t *testing.T) {
	svc, _, tenantID, participantID := newNoteSvc(t)
	ctx := tctx(tenantID)

	for _, d := range []string{"2026-01-10", "2026-01-15", "2026-01-20"} {
		if _, err := svc.Create(ctx, repository.NoteInput{ParticipantID: participantID, ServiceDate: d, Body: "n"}); err != nil {
			t.Fatalf("Create %s: %v", d, err)
		}
	}
	rng, err := svc.ListParticipant(ctx, participantID, "2026-01-15", "2026-01-20")
	if err != nil {
		t.Fatalf("ListParticipant range: %v", err)
	}
	if len(rng) != 2 {
		t.Fatalf("range [15,20] = %d, want 2", len(rng))
	}
}

func TestNoteUpdateUnknownNoEvent(t *testing.T) {
	svc, hub, tenantID, participantID := newNoteSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	n, err := svc.Update(ctx, 99999, repository.NoteInput{
		ParticipantID: participantID, ServiceDate: "2026-01-15", Body: "x",
	})
	if err != nil || n != nil {
		t.Fatalf("Update unknown = %+v err=%v, want nil/nil", n, err)
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected for unknown update, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

func TestNoteDelete(t *testing.T) {
	svc, hub, tenantID, participantID := newNoteSvc(t)
	ctx := tctx(tenantID)

	n, err := svc.Create(ctx, repository.NoteInput{ParticipantID: participantID, ServiceDate: "2026-01-15", Body: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	if err := svc.Delete(ctx, n.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "note" || e.ID != n.ID || e.Action != "delete" {
			t.Fatalf("event=%+v want note/%d/delete", e, n.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Delete")
	}
	if got, _ := svc.Get(ctx, n.ID); got != nil {
		t.Fatalf("note present after delete: %+v", got)
	}
}

func TestNoteBillSetsAndBroadcasts(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	invID := seedNoteInvoice(t, conn, tenantID, participantID)
	hub := realtime.NewHub()
	svc := NewNoteService(conn, hub)
	ctx := tctx(tenantID)

	n, err := svc.Create(ctx, repository.NoteInput{ParticipantID: participantID, ServiceDate: "2026-01-15", Body: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	if err := svc.Bill(ctx, invID, []int64{n.ID}); err != nil {
		t.Fatalf("Bill: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "note" || e.Action != "bill" {
			t.Fatalf("event=%+v want note/bill", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Bill")
	}
	got, _ := svc.Get(ctx, n.ID)
	if got == nil || got.BilledID == nil || *got.BilledID != invID {
		t.Fatalf("billed invoice id = %+v, want %d", got.BilledID, invID)
	}
}

func TestNoteBillRejectsCrossTenantInvoice(t *testing.T) {
	conn := newTestDB(t)

	// Tenant A: owns an invoice.
	tenantA := seedTenant(t, conn)
	participantA := seedParticipant(t, conn, tenantA)
	invA := seedNoteInvoice(t, conn, tenantA, participantA)

	// Tenant B: owns a note.
	tenantB := seedTenant(t, conn)
	participantB := seedParticipant(t, conn, tenantB)
	hub := realtime.NewHub()
	svc := NewNoteService(conn, hub)
	ctxB := tctx(tenantB)

	n, err := svc.Create(ctxB, repository.NoteInput{ParticipantID: participantB, ServiceDate: "2026-01-15", Body: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantB)
	defer unsub()

	// B tries to bill its note onto A's invoice — must be rejected.
	if err := svc.Bill(ctxB, invA, []int64{n.ID}); err == nil {
		t.Fatal("Bill cross-tenant invoice: want error, got nil")
	}

	// No bill event should fire.
	select {
	case e := <-ch:
		t.Fatalf("no event expected on rejected cross-tenant Bill, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}

	// B's note must remain unbilled.
	got, _ := svc.Get(ctxB, n.ID)
	if got == nil || got.BilledID != nil {
		t.Fatalf("note must remain unbilled after rejected cross-tenant Bill: %+v", got)
	}
}

func TestNoteBillEmptyNoEvent(t *testing.T) {
	svc, hub, tenantID, _ := newNoteSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.Bill(tctx(tenantID), 55, nil); err != nil {
		t.Fatalf("Bill empty: %v", err)
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on empty Bill, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
