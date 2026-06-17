package repository

import (
	"context"
	"testing"
)

func TestShiftCreateRoundTrip(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	uid := seedUser(t, conn, tid)
	repo := NewShifts(conn)
	ctx := context.Background()

	in := ShiftInput{
		ParticipantID: pid, ServiceDate: "2026-01-15", StartTime: "09:00", EndTime: "11:30",
		Hours: 2.5, Km: 12.5,
		Measures: []Measure{{Label: "Personal care", Value: 2.5, Unit: "hr", Code: "01_011_0107_1_1"}},
		Note:     "Support visit",
		Tags:     []string{"personal-care", "transport"},
	}
	s, err := repo.Create(ctx, tid, &uid, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s == nil || s.ID == 0 || s.UUID == "" {
		t.Fatalf("Create = %+v", s)
	}
	if s.ParticipantID != pid || s.ServiceDate != "2026-01-15" || s.StartTime != "09:00" || s.EndTime != "11:30" {
		t.Fatalf("scalar fields not round-tripped: %+v", s)
	}
	if s.Hours != 2.5 || s.Km != 12.5 || s.Note != "Support visit" {
		t.Fatalf("hours/km/note not round-tripped: %+v", s)
	}
	if len(s.Measures) != 1 || s.Measures[0].Label != "Personal care" || s.Measures[0].Value != 2.5 ||
		s.Measures[0].Unit != "hr" || s.Measures[0].Code != "01_011_0107_1_1" {
		t.Fatalf("measures not round-tripped: %+v", s.Measures)
	}
	if len(s.Tags) != 2 || s.Tags[0] != "personal-care" || s.Tags[1] != "transport" {
		t.Fatalf("tags not round-tripped: %+v", s.Tags)
	}
	if s.AuthorUserID == nil || *s.AuthorUserID != uid {
		t.Fatalf("author not round-tripped: %+v want %d", s.AuthorUserID, uid)
	}
	if s.InvoiceID != nil {
		t.Fatalf("InvoiceID should be nil on a fresh shift: %+v", s.InvoiceID)
	}
	if s.Status != "recorded" {
		t.Fatalf("status default = %q, want recorded", s.Status)
	}
}

func TestShiftStatusDefaultsRecorded(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	// Empty status → recorded.
	s, err := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: pid, ServiceDate: "2026-01-15"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.Status != "recorded" {
		t.Fatalf("default status = %q, want recorded", s.Status)
	}
	// Empty measures/tags decode to non-nil empty slices.
	if s.Measures == nil || len(s.Measures) != 0 {
		t.Fatalf("empty measures must be non-nil empty: %+v", s.Measures)
	}
	if s.Tags == nil || len(s.Tags) != 0 {
		t.Fatalf("empty tags must be non-nil empty: %+v", s.Tags)
	}

	// Explicit status honoured.
	sched, err := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: pid, ServiceDate: "2026-01-16", Status: "scheduled"})
	if err != nil {
		t.Fatalf("Create scheduled: %v", err)
	}
	if sched.Status != "scheduled" {
		t.Fatalf("explicit status = %q, want scheduled", sched.Status)
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
		{"zero participant", ShiftInput{ParticipantID: 0, ServiceDate: "2026-01-15"}},
		{"empty serviceDate", ShiftInput{ParticipantID: pid, ServiceDate: ""}},
		{"malformed date", ShiftInput{ParticipantID: pid, ServiceDate: "2026-6-9"}},
		{"negative hours", ShiftInput{ParticipantID: pid, ServiceDate: "2026-01-15", Hours: -1}},
		{"negative km", ShiftInput{ParticipantID: pid, ServiceDate: "2026-01-15", Km: -1}},
	}
	for _, c := range cases {
		if s, err := repo.Create(ctx, tid, nil, c.in); err == nil {
			t.Fatalf("%s: want error, got nil (shift=%+v)", c.name, s)
		}
	}
	// Nothing inserted.
	all, err := repo.ListParticipant(ctx, tid, pid, "", "")
	if err != nil {
		t.Fatalf("ListParticipant: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("rejected creates left rows: %+v", all)
	}
}

func TestShiftGetFoundAbsentAndIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	pa := seedParticipant(t, conn, a, "Jane")
	repo := NewShifts(conn)
	ctx := context.Background()

	s, err := repo.Create(ctx, a, nil, ShiftInput{ParticipantID: pa, ServiceDate: "2026-01-15"})
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

func TestShiftListParticipantRange(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	other := seedParticipant(t, conn, tid, "Other")
	repo := NewShifts(conn)
	ctx := context.Background()

	for _, d := range []string{"2026-01-10", "2026-01-15", "2026-01-20"} {
		if _, err := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: pid, ServiceDate: d}); err != nil {
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
		t.Fatalf("range [15,20] inclusive = %d, want 2", len(rng))
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

	s, err := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: pid, ServiceDate: "2026-01-15", Status: "scheduled"})
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
}

func TestShiftSetInvoiceAndClear(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	invID := seedInvoice(t, conn, tid, pid, 100)
	repo := NewShifts(conn)
	ctx := context.Background()

	s, err := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: pid, ServiceDate: "2026-01-15"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SetInvoice(ctx, tid, s.ID, invID, "drafted"); err != nil {
		t.Fatalf("SetInvoice: %v", err)
	}
	got, _ := repo.Get(ctx, tid, s.ID)
	if got == nil || got.Status != "drafted" || got.InvoiceID == nil || *got.InvoiceID != invID {
		t.Fatalf("after SetInvoice = %+v, want drafted + invoice %d", got, invID)
	}

	// ClearForInvoice reverts to recorded + nil invoice.
	if err := repo.ClearForInvoice(ctx, tid, invID); err != nil {
		t.Fatalf("ClearForInvoice: %v", err)
	}
	got, _ = repo.Get(ctx, tid, s.ID)
	if got == nil || got.Status != "recorded" || got.InvoiceID != nil {
		t.Fatalf("after ClearForInvoice = %+v, want recorded + nil invoice", got)
	}
}

func TestShiftSetStatusForInvoice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	invID := seedInvoice(t, conn, tid, pid, 100)
	repo := NewShifts(conn)
	ctx := context.Background()

	s, _ := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: pid, ServiceDate: "2026-01-15"})
	if err := repo.SetInvoice(ctx, tid, s.ID, invID, "drafted"); err != nil {
		t.Fatalf("SetInvoice: %v", err)
	}
	if err := repo.SetStatusForInvoice(ctx, tid, invID, "sent"); err != nil {
		t.Fatalf("SetStatusForInvoice: %v", err)
	}
	got, _ := repo.Get(ctx, tid, s.ID)
	if got == nil || got.Status != "sent" {
		t.Fatalf("after SetStatusForInvoice = %+v, want sent", got)
	}
}

func TestShiftUnbilledByParticipant(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	p1 := seedParticipant(t, conn, tid, "Jane")
	p2 := seedParticipant(t, conn, tid, "John")
	invID := seedInvoice(t, conn, tid, p1, 100)
	repo := NewShifts(conn)
	ctx := context.Background()

	// p1: three recorded unbilled across a date range.
	for _, d := range []string{"2026-01-10", "2026-01-15", "2026-01-20"} {
		if _, err := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: p1, ServiceDate: d}); err != nil {
			t.Fatalf("Create %s: %v", d, err)
		}
	}
	// p1: one drafted (billed) shift — excluded.
	billed, _ := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: p1, ServiceDate: "2026-01-25"})
	if err := repo.SetInvoice(ctx, tid, billed.ID, invID, "drafted"); err != nil {
		t.Fatalf("SetInvoice: %v", err)
	}
	// p2: one recorded unbilled.
	if _, err := repo.Create(ctx, tid, nil, ShiftInput{ParticipantID: p2, ServiceDate: "2026-02-01"}); err != nil {
		t.Fatalf("Create p2: %v", err)
	}

	aggs, err := repo.UnbilledByParticipant(ctx, tid)
	if err != nil {
		t.Fatalf("UnbilledByParticipant: %v", err)
	}
	byPID := map[int64]Agg{}
	for _, a := range aggs {
		byPID[a.ParticipantID] = a
	}
	if a, ok := byPID[p1]; !ok || a.Count != 3 || a.From != "2026-01-10" || a.To != "2026-01-20" {
		t.Fatalf("p1 agg = %+v, want count 3 [10,20]", a)
	}
	if a, ok := byPID[p2]; !ok || a.Count != 1 || a.From != "2026-02-01" || a.To != "2026-02-01" {
		t.Fatalf("p2 agg = %+v, want count 1 [01,01]", a)
	}

	// ListRecordedUnbilled returns p1's three unbilled shifts.
	un, err := repo.ListRecordedUnbilled(ctx, tid, p1)
	if err != nil {
		t.Fatalf("ListRecordedUnbilled: %v", err)
	}
	if len(un) != 3 {
		t.Fatalf("ListRecordedUnbilled = %d, want 3", len(un))
	}
}
