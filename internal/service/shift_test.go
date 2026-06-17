package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

func newShiftSvc(t *testing.T) (*ShiftService, *realtime.Hub, int64, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	hub := realtime.NewHub()
	return NewShiftService(conn, hub), hub, tenantID, participantID
}

func TestShiftCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantID := newShiftSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	s, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: participantID, ServiceDate: "2026-01-15"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s == nil {
		t.Fatal("Create returned nil shift")
	}
	select {
	case e := <-ch:
		if e.Entity != "shift" || e.ID != s.ID || e.Action != "create" {
			t.Fatalf("event=%+v want shift/%d/create", e, s.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestShiftCreateAttributesAuthor(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	uid := seedNoteUser(t, conn, tenantID)
	svc := NewShiftService(conn, realtime.NewHub())
	ctx := reqctx.WithUser(tctx(tenantID), uid)

	s, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: participantID, ServiceDate: "2026-01-15"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.AuthorUserID == nil || *s.AuthorUserID != uid {
		t.Fatalf("author not attributed: %+v want %d", s.AuthorUserID, uid)
	}
}

func TestShiftListParticipantRange(t *testing.T) {
	svc, _, tenantID, participantID := newShiftSvc(t)
	ctx := tctx(tenantID)

	for _, d := range []string{"2026-01-10", "2026-01-15", "2026-01-20"} {
		if _, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: participantID, ServiceDate: d}); err != nil {
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

func TestShiftUpdateStatusBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantID := newShiftSvc(t)
	ctx := tctx(tenantID)

	s, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: participantID, ServiceDate: "2026-01-15", Status: "scheduled"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	if err := svc.UpdateStatus(ctx, s.ID, "recorded"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "shift" || e.ID != s.ID {
			t.Fatalf("event=%+v want shift/%d", e, s.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after UpdateStatus")
	}
	got, _ := svc.Get(ctx, s.ID)
	if got == nil || got.Status != "recorded" {
		t.Fatalf("status after UpdateStatus = %+v, want recorded", got)
	}
}

func TestShiftDeleteBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantID := newShiftSvc(t)
	ctx := tctx(tenantID)

	s, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: participantID, ServiceDate: "2026-01-15"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	if err := svc.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "shift" || e.ID != s.ID || e.Action != "delete" {
			t.Fatalf("event=%+v want shift/%d/delete", e, s.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Delete")
	}
	if got, _ := svc.Get(ctx, s.ID); got != nil {
		t.Fatalf("shift present after delete: %+v", got)
	}
}

func TestShiftToRecord(t *testing.T) {
	svc, _, tenantID, participantID := newShiftSvc(t)
	ctx := tctx(tenantID)

	if _, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: participantID, ServiceDate: "2026-01-15", Status: "scheduled"}); err != nil {
		t.Fatalf("Create scheduled: %v", err)
	}
	if _, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: participantID, ServiceDate: "2026-01-16"}); err != nil {
		t.Fatalf("Create recorded: %v", err)
	}
	sched, err := svc.ToRecord(ctx)
	if err != nil {
		t.Fatalf("ToRecord: %v", err)
	}
	if len(sched) != 1 || sched[0].Status != "scheduled" {
		t.Fatalf("ToRecord = %+v, want 1 scheduled", sched)
	}
}

func TestShiftSuggestions(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	p1 := seedParticipant(t, conn, tenantID)
	p2 := seedParticipant(t, conn, tenantID)
	svc := NewShiftService(conn, realtime.NewHub())
	ctx := tctx(tenantID)

	var p1IDs []int64
	for _, d := range []string{"2026-01-10", "2026-01-15", "2026-01-20"} {
		s, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: p1, ServiceDate: d})
		if err != nil {
			t.Fatalf("Create p1 %s: %v", d, err)
		}
		p1IDs = append(p1IDs, s.ID)
	}
	if _, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: p2, ServiceDate: "2026-02-01"}); err != nil {
		t.Fatalf("Create p2: %v", err)
	}

	sug, err := svc.Suggestions(ctx)
	if err != nil {
		t.Fatalf("Suggestions: %v", err)
	}
	byPID := map[int64]Suggestion{}
	for _, s := range sug {
		byPID[s.ParticipantID] = s
	}
	s1, ok := byPID[p1]
	if !ok || s1.Count != 3 || s1.From != "2026-01-10" || s1.To != "2026-01-20" {
		t.Fatalf("p1 suggestion = %+v, want count 3 [10,20]", s1)
	}
	if len(s1.IDs) != 3 {
		t.Fatalf("p1 suggestion IDs = %v, want 3", s1.IDs)
	}
	for _, id := range p1IDs {
		if !containsID(s1.IDs, id) {
			t.Fatalf("suggestion missing shift id %d: %v", id, s1.IDs)
		}
	}
	s2, ok := byPID[p2]
	if !ok || s2.Count != 1 || len(s2.IDs) != 1 {
		t.Fatalf("p2 suggestion = %+v, want count 1", s2)
	}
}

func TestShiftMarkDrafted(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	invID := seedNoteInvoice(t, conn, tenantID, participantID)
	hub := realtime.NewHub()
	svc := NewShiftService(conn, hub)
	ctx := tctx(tenantID)

	s, err := svc.Create(ctx, repository.ShiftInput{ParticipantID: participantID, ServiceDate: "2026-01-15"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	if err := svc.MarkDrafted(ctx, invID, []int64{s.ID}); err != nil {
		t.Fatalf("MarkDrafted: %v", err)
	}
	select {
	case e := <-ch:
		if e.Entity != "shift" {
			t.Fatalf("event=%+v want shift", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after MarkDrafted")
	}
	got, _ := svc.Get(ctx, s.ID)
	if got == nil || got.Status != "drafted" || got.InvoiceID == nil || *got.InvoiceID != invID {
		t.Fatalf("after MarkDrafted = %+v, want drafted + invoice %d", got, invID)
	}
}

func TestShiftMarkDraftedRejectsCrossTenantInvoice(t *testing.T) {
	conn := newTestDB(t)

	tenantA := seedTenant(t, conn)
	participantA := seedParticipant(t, conn, tenantA)
	invA := seedNoteInvoice(t, conn, tenantA, participantA)

	tenantB := seedTenant(t, conn)
	participantB := seedParticipant(t, conn, tenantB)
	svc := NewShiftService(conn, realtime.NewHub())
	ctxB := tctx(tenantB)

	s, err := svc.Create(ctxB, repository.ShiftInput{ParticipantID: participantB, ServiceDate: "2026-01-15"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.MarkDrafted(ctxB, invA, []int64{s.ID}); err == nil {
		t.Fatal("MarkDrafted cross-tenant invoice: want error, got nil")
	}
	got, _ := svc.Get(ctxB, s.ID)
	if got == nil || got.Status != "recorded" || got.InvoiceID != nil {
		t.Fatalf("shift must remain recorded after rejected cross-tenant MarkDrafted: %+v", got)
	}
}
