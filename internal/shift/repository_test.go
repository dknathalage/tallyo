package shift

import (
	"context"
	"testing"
)

func sampleShiftInput(pid int64) ShiftInput {
	return ShiftInput{
		ParticipantID: pid,
		ServiceDate:   "2026-01-15",
		Note:          "Supported community access",
		Tags:          []string{"community", "transport"},
	}
}

func TestShiftCreateRoundTrip(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	uid := seedUser(t, conn, tid)
	repo := NewShifts(conn)
	ctx := context.Background()

	s, err := repo.Create(ctx, tid, &uid, sampleShiftInput(pid))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s == nil || s.ID == 0 || s.ParticipantID != pid || s.ServiceDate != "2026-01-15" {
		t.Fatalf("Create = %+v", s)
	}
	if s.Note != "Supported community access" {
		t.Fatalf("scalar fields not round-tripped: %+v", s)
	}
	if len(s.Tags) != 2 || s.Tags[0] != "community" || s.Tags[1] != "transport" {
		t.Fatalf("tags not round-tripped: %+v", s.Tags)
	}
	if s.AuthorUserID == nil || *s.AuthorUserID != uid {
		t.Fatalf("author not round-tripped: %+v want %d", s.AuthorUserID, uid)
	}
	if s.InvoiceID != nil {
		t.Fatalf("InvoiceID should be nil on a fresh shift: %+v", s.InvoiceID)
	}
	if s.Status != "recorded" {
		t.Fatalf("default status = %q, want recorded", s.Status)
	}
}

func TestShiftCreateEmptyTagsNeverNil(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	s, err := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: pid, ServiceDate: "2026-01-16"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.Tags == nil || len(s.Tags) != 0 {
		t.Fatalf("tags must be non-nil empty, got %+v", s.Tags)
	}
}

func TestShiftCreateStatusOverride(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	in := sampleShiftInput(pid)
	in.Status = "scheduled"
	s, err := repo.Create(ctx, tid, nil, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.Status != "scheduled" {
		t.Fatalf("status = %q, want scheduled", s.Status)
	}
}

func TestShiftCreateRejectsInvalid(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	cases := []struct {
		name string
		in   ShiftInput
	}{
		{"empty serviceDate", ShiftInput{ParticipantID: pid, ServiceDate: ""}},
		{"malformed serviceDate", ShiftInput{ParticipantID: pid, ServiceDate: "2026-6-9"}},
		{"zero participant", ShiftInput{ParticipantID: 0, ServiceDate: "2026-01-15"}},
	}
	for _, c := range cases {
		if s, err := repo.Create(ctx, tid, nil, c.in); err == nil {
			t.Fatalf("%s: want error, got nil (shift=%+v)", c.name, s)
		}
	}
}

func TestShiftGetFoundAbsentAndIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	pa := seedParticipant(t, conn, a, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	s, err := repo.Create(ctx, a, nil, sampleShiftInput(pa))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.Get(ctx, a, s.ID)
	if err != nil || got == nil || got.ID != s.ID {
		t.Fatalf("Get found = %+v err=%v", got, err)
	}
	if miss, err := repo.Get(ctx, a, s.ID+999); err != nil || miss != nil {
		t.Fatalf("Get absent = %+v err=%v", miss, err)
	}
	if leak, _ := repo.Get(ctx, b, s.ID); leak != nil {
		t.Fatalf("tenant B read tenant A's shift: %+v", leak)
	}
}

func TestShiftListParticipantRangeRepo(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	other := seedParticipant(t, conn, tid, "Other")
	repo := NewShifts(conn)
	ctx := context.Background()

	for _, d := range []string{"2026-01-10", "2026-01-15", "2026-01-20"} {
		in := sampleShiftInput(pid)
		in.ServiceDate = d
		if _, err := repo.Create(ctx, tid, nil, in); err != nil {
			t.Fatalf("Create %s: %v", d, err)
		}
	}

	all, err := repo.ListParticipant(ctx, tid, pid, "", "")
	if err != nil || len(all) != 3 {
		t.Fatalf("ListParticipant all = %d err=%v", len(all), err)
	}

	rng, err := repo.ListParticipant(ctx, tid, pid, "2026-01-15", "2026-01-20")
	if err != nil {
		t.Fatalf("ListParticipant range: %v", err)
	}
	if len(rng) != 2 {
		t.Fatalf("range [15,20] inclusive = %d, want 2: %+v", len(rng), rng)
	}

	none, err := repo.ListParticipant(ctx, tid, other, "", "")
	if err != nil {
		t.Fatalf("ListParticipant none: %v", err)
	}
	if none == nil || len(none) != 0 {
		t.Fatalf("no-shifts list must be non-nil empty, got %+v", none)
	}
}

func TestShiftUpdateStatus(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	in := sampleShiftInput(pid)
	in.Status = "scheduled"
	s, err := repo.Create(ctx, tid, nil, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.UpdateStatus(ctx, tid, s.ID, "recorded"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := repo.Get(ctx, tid, s.ID)
	if got == nil || got.Status != "recorded" {
		t.Fatalf("status after UpdateStatus = %+v, want recorded", got)
	}

	scheduled, err := repo.ListScheduled(ctx, tid)
	if err != nil {
		t.Fatalf("ListScheduled: %v", err)
	}
	if len(scheduled) != 0 {
		t.Fatalf("ListScheduled = %d, want 0 after flip to recorded", len(scheduled))
	}
}

func TestShiftSetInvoiceAndClear(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	s, err := repo.Create(ctx, tid, nil, sampleShiftInput(pid))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	invID := seedInvoice(t, conn, tid, pid, 100)

	if err := repo.SetInvoice(ctx, tid, s.ID, invID, "drafted"); err != nil {
		t.Fatalf("SetInvoice: %v", err)
	}
	got, _ := repo.Get(ctx, tid, s.ID)
	if got == nil || got.Status != "drafted" || got.InvoiceID == nil || *got.InvoiceID != invID {
		t.Fatalf("after SetInvoice = %+v, want drafted+invoice %d", got, invID)
	}

	if err := repo.ClearForInvoice(ctx, tid, invID); err != nil {
		t.Fatalf("ClearForInvoice: %v", err)
	}
	got, _ = repo.Get(ctx, tid, s.ID)
	if got == nil || got.Status != "recorded" || got.InvoiceID != nil {
		t.Fatalf("after ClearForInvoice = %+v, want recorded+nil invoice", got)
	}
}

func TestShiftSetStatusForInvoice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	s, _ := repo.Create(ctx, tid, nil, sampleShiftInput(pid))
	invID := seedInvoice(t, conn, tid, pid, 100)
	if err := repo.SetInvoice(ctx, tid, s.ID, invID, "drafted"); err != nil {
		t.Fatalf("SetInvoice: %v", err)
	}
	if err := repo.SetStatusForInvoice(ctx, tid, invID, "sent"); err != nil {
		t.Fatalf("SetStatusForInvoice: %v", err)
	}
	got, _ := repo.Get(ctx, tid, s.ID)
	if got == nil || got.Status != "sent" || got.InvoiceID == nil || *got.InvoiceID != invID {
		t.Fatalf("after SetStatusForInvoice = %+v, want sent and invoice still linked", got)
	}
}

func TestShiftListRecordedUnbilled(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	// Two recorded unbilled.
	r1, _ := repo.Create(ctx, tid, nil, sampleShiftInput(pid))
	r2in := sampleShiftInput(pid)
	r2in.ServiceDate = "2026-01-16"
	r2, _ := repo.Create(ctx, tid, nil, r2in)

	// One scheduled (excluded).
	schedIn := sampleShiftInput(pid)
	schedIn.Status = "scheduled"
	if _, err := repo.Create(ctx, tid, nil, schedIn); err != nil {
		t.Fatalf("Create scheduled: %v", err)
	}

	// One billed (excluded).
	billed, _ := repo.Create(ctx, tid, nil, sampleShiftInput(pid))
	invID := seedInvoice(t, conn, tid, pid, 100)
	if err := repo.SetInvoice(ctx, tid, billed.ID, invID, "drafted"); err != nil {
		t.Fatalf("SetInvoice: %v", err)
	}

	unbilled, err := repo.ListRecordedUnbilled(ctx, tid, pid)
	if err != nil {
		t.Fatalf("ListRecordedUnbilled: %v", err)
	}
	if len(unbilled) != 2 {
		t.Fatalf("ListRecordedUnbilled = %d, want 2: %+v", len(unbilled), unbilled)
	}
	ids := map[int64]bool{unbilled[0].ID: true, unbilled[1].ID: true}
	if !ids[r1.ID] || !ids[r2.ID] {
		t.Fatalf("unbilled ids = %+v, want %d and %d", ids, r1.ID, r2.ID)
	}
}

func TestShiftUnbilledByParticipant(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	p1 := seedParticipant(t, conn, tid, "Jane")
	p2 := seedParticipant(t, conn, tid, "John")
	repo := NewShifts(conn)
	ctx := context.Background()

	for _, d := range []string{"2026-01-10", "2026-01-20"} {
		in := sampleShiftInput(p1)
		in.ServiceDate = d
		if _, err := repo.Create(ctx, tid, nil, in); err != nil {
			t.Fatalf("Create p1 %s: %v", d, err)
		}
	}
	in2 := sampleShiftInput(p2)
	in2.ServiceDate = "2026-02-01"
	if _, err := repo.Create(ctx, tid, nil, in2); err != nil {
		t.Fatalf("Create p2: %v", err)
	}

	aggs, err := repo.UnbilledByParticipant(ctx, tid)
	if err != nil {
		t.Fatalf("UnbilledByParticipant: %v", err)
	}
	if len(aggs) != 2 {
		t.Fatalf("aggs = %d, want 2: %+v", len(aggs), aggs)
	}
	byPID := map[int64]UnbilledAgg{}
	for _, a := range aggs {
		byPID[a.ParticipantID] = a
	}
	if a := byPID[p1]; a.Count != 2 || a.From != "2026-01-10" || a.To != "2026-01-20" {
		t.Fatalf("p1 agg = %+v, want count 2 from 01-10 to 01-20", a)
	}
	if a := byPID[p2]; a.Count != 1 || a.From != "2026-02-01" || a.To != "2026-02-01" {
		t.Fatalf("p2 agg = %+v, want count 1 from/to 02-01", a)
	}
}

func TestShiftUpdateRepo(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	s, err := repo.Create(ctx, tid, nil, sampleShiftInput(pid))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	in := sampleShiftInput(pid)
	in.ServiceDate = "2026-01-18"
	in.Note = "updated note"
	in.Tags = []string{"updated"}
	up, err := repo.Update(ctx, tid, s.ID, in)
	if err != nil || up == nil || up.ServiceDate != "2026-01-18" || up.Note != "updated note" {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if len(up.Tags) != 1 || up.Tags[0] != "updated" {
		t.Fatalf("Update tags = %+v", up.Tags)
	}

	if miss, err := repo.Update(ctx, tid, s.ID+999, in); err != nil || miss != nil {
		t.Fatalf("Update unknown = %+v err=%v", miss, err)
	}
}

func TestShiftDeleteRepo(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	s, err := repo.Create(ctx, tid, nil, sampleShiftInput(pid))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, tid, s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, s.ID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}
