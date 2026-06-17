package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

func fptr(f float64) *float64 { return &f }

// seedUser inserts a minimal user so notes can satisfy the author_user_id FK.
func seedUser(t *testing.T, conn *sql.DB, tenantID int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	u, err := gen.New(conn).CreateUser(context.Background(), gen.CreateUserParams{
		Uuid: uuid.NewString(), TenantID: tenantID, Email: uuid.NewString() + "@x.com",
		PasswordHash: "x", Name: "U", Role: "member", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedUser: %v", err)
	}
	return u.ID
}

func TestNoteCreateRoundTrip(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	uid := seedUser(t, conn, tid)
	repo := NewNotes(conn)
	ctx := context.Background()

	// With optional km/hours and an author present.
	n, err := repo.Create(ctx, tid, &uid, NoteInput{
		ParticipantID: pid, ServiceDate: "2026-01-15", Body: "Support visit",
		TransportKm: fptr(12.5), SupportHours: fptr(2),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n == nil || n.ID == 0 || n.Body != "Support visit" || n.ServiceDate != "2026-01-15" || n.ParticipantID != pid {
		t.Fatalf("Create = %+v", n)
	}
	if n.TransportKm == nil || *n.TransportKm != 12.5 || n.SupportHours == nil || *n.SupportHours != 2 {
		t.Fatalf("optional fields not round-tripped: %+v", n)
	}
	if n.AuthorUserID == nil || *n.AuthorUserID != uid {
		t.Fatalf("author not round-tripped: %+v want %d", n.AuthorUserID, uid)
	}
	if n.BilledID != nil {
		t.Fatalf("BilledID should be nil on a fresh note: %+v", n.BilledID)
	}

	// Optional km/hours omitted → nil.
	n2, err := repo.Create(ctx, tid, nil, NoteInput{
		ParticipantID: pid, ServiceDate: "2026-01-16", Body: "Phone check-in",
	})
	if err != nil {
		t.Fatalf("Create no-optionals: %v", err)
	}
	if n2.TransportKm != nil || n2.SupportHours != nil {
		t.Fatalf("omitted optionals should be nil: %+v", n2)
	}
}

func TestNoteCreateRejectsInvalid(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewNotes(conn)
	ctx := context.Background()

	cases := []struct {
		name string
		in   NoteInput
	}{
		{"empty body", NoteInput{ParticipantID: pid, ServiceDate: "2026-01-15", Body: ""}},
		{"empty serviceDate", NoteInput{ParticipantID: pid, ServiceDate: "", Body: "x"}},
		{"zero participant", NoteInput{ParticipantID: 0, ServiceDate: "2026-01-15", Body: "x"}},
		{"negative km", NoteInput{ParticipantID: pid, ServiceDate: "2026-01-15", Body: "x", TransportKm: fptr(-1)}},
		{"negative hours", NoteInput{ParticipantID: pid, ServiceDate: "2026-01-15", Body: "x", SupportHours: fptr(-1)}},
	}
	for _, c := range cases {
		if _, err := repo.Create(ctx, tid, nil, c.in); err == nil {
			t.Fatalf("%s: want error, got nil", c.name)
		}
	}
}

func TestNoteCreateRejectsMalformedDate(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewNotes(conn)
	ctx := context.Background()

	// Non-ISO service dates are rejected and create no row.
	for _, bad := range []string{"2026-6-9", "not-a-date"} {
		n, err := repo.Create(ctx, tid, nil, NoteInput{ParticipantID: pid, ServiceDate: bad, Body: "x"})
		if err == nil {
			t.Fatalf("Create %q: want error, got nil (note=%+v)", bad, n)
		}
		if n != nil {
			t.Fatalf("Create %q: no note should be returned, got %+v", bad, n)
		}
	}

	// Sanity: no rows were inserted by the rejected creates.
	all, err := repo.ListParticipant(ctx, tid, pid, "", "")
	if err != nil {
		t.Fatalf("ListParticipant: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("rejected creates left rows behind: %+v", all)
	}

	// A valid ISO date still succeeds (guard against over-rejection).
	good, err := repo.Create(ctx, tid, nil, NoteInput{ParticipantID: pid, ServiceDate: "2026-06-09", Body: "ok"})
	if err != nil || good == nil || good.ServiceDate != "2026-06-09" {
		t.Fatalf("valid date Create = %+v err=%v", good, err)
	}

	// Update also rejects a malformed date.
	if _, err := repo.Update(ctx, tid, good.ID, NoteInput{ParticipantID: pid, ServiceDate: "2026-6-9", Body: "x"}); err == nil {
		t.Fatal("Update with malformed date: want error, got nil")
	}
}

func TestNoteGetFoundAbsentAndIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	pa := seedParticipant(t, conn, a, "Jane")
	repo := NewNotes(conn)
	ctx := context.Background()

	n, err := repo.Create(ctx, a, nil, NoteInput{ParticipantID: pa, ServiceDate: "2026-01-15", Body: "A note"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Found under tenant A.
	got, err := repo.Get(ctx, a, n.ID)
	if err != nil || got == nil || got.ID != n.ID {
		t.Fatalf("Get found = %+v err=%v", got, err)
	}
	// Absent id → nil, nil.
	if miss, err := repo.Get(ctx, a, n.ID+999); err != nil || miss != nil {
		t.Fatalf("Get absent = %+v err=%v", miss, err)
	}
	// Tenant isolation: invisible to tenant B.
	if leak, _ := repo.Get(ctx, b, n.ID); leak != nil {
		t.Fatalf("tenant B read tenant A's note: %+v", leak)
	}
}

func TestNoteListParticipant(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	other := seedParticipant(t, conn, tid, "Other")
	repo := NewNotes(conn)
	ctx := context.Background()

	for _, d := range []string{"2026-01-10", "2026-01-15", "2026-01-20"} {
		if _, err := repo.Create(ctx, tid, nil, NoteInput{ParticipantID: pid, ServiceDate: d, Body: "n " + d}); err != nil {
			t.Fatalf("Create %s: %v", d, err)
		}
	}

	// Empty range returns all.
	all, err := repo.ListParticipant(ctx, tid, pid, "", "")
	if err != nil || len(all) != 3 {
		t.Fatalf("ListParticipant all = %d err=%v", len(all), err)
	}

	// Range filters inclusive by service_date.
	rng, err := repo.ListParticipant(ctx, tid, pid, "2026-01-15", "2026-01-20")
	if err != nil {
		t.Fatalf("ListParticipant range: %v", err)
	}
	if len(rng) != 2 {
		t.Fatalf("range [15,20] inclusive = %d, want 2: %+v", len(rng), rng)
	}

	// Participant with no notes returns a non-nil empty slice.
	none, err := repo.ListParticipant(ctx, tid, other, "", "")
	if err != nil {
		t.Fatalf("ListParticipant none: %v", err)
	}
	if none == nil || len(none) != 0 {
		t.Fatalf("no-notes list must be non-nil empty, got %+v", none)
	}
}

func TestNoteUpdate(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewNotes(conn)
	ctx := context.Background()

	n, err := repo.Create(ctx, tid, nil, NoteInput{ParticipantID: pid, ServiceDate: "2026-01-15", Body: "before"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	up, err := repo.Update(ctx, tid, n.ID, NoteInput{
		ParticipantID: pid, ServiceDate: "2026-01-16", Body: "after", SupportHours: fptr(3),
	})
	if err != nil || up == nil || up.Body != "after" || up.ServiceDate != "2026-01-16" {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if up.SupportHours == nil || *up.SupportHours != 3 {
		t.Fatalf("Update did not set supportHours: %+v", up)
	}

	// Unknown id → nil, nil.
	if miss, err := repo.Update(ctx, tid, n.ID+999, NoteInput{ParticipantID: pid, ServiceDate: "2026-01-16", Body: "x"}); err != nil || miss != nil {
		t.Fatalf("Update unknown = %+v err=%v", miss, err)
	}
	// Empty body rejected.
	if _, err := repo.Update(ctx, tid, n.ID, NoteInput{ParticipantID: pid, ServiceDate: "2026-01-16", Body: ""}); err == nil {
		t.Fatal("Update empty body must error")
	}
}

func TestNoteDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewNotes(conn)
	ctx := context.Background()

	n, err := repo.Create(ctx, tid, nil, NoteInput{ParticipantID: pid, ServiceDate: "2026-01-15", Body: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, tid, n.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, n.ID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestNoteMarkBilled(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewNotes(conn)
	ctx := context.Background()

	n1, _ := repo.Create(ctx, tid, nil, NoteInput{ParticipantID: pid, ServiceDate: "2026-01-15", Body: "a"})
	n2, _ := repo.Create(ctx, tid, nil, NoteInput{ParticipantID: pid, ServiceDate: "2026-01-16", Body: "b"})

	// Empty ids is a no-op.
	if err := repo.MarkBilled(ctx, tid, nil, nil); err != nil {
		t.Fatalf("MarkBilled empty: %v", err)
	}

	// Set BilledID for the given ids (a real invoice to satisfy the FK).
	invID := seedInvoice(t, conn, tid, pid, 100)
	if err := repo.MarkBilled(ctx, tid, &invID, []int64{n1.ID, n2.ID}); err != nil {
		t.Fatalf("MarkBilled set: %v", err)
	}
	for _, id := range []int64{n1.ID, n2.ID} {
		got, _ := repo.Get(ctx, tid, id)
		if got == nil || got.BilledID == nil || *got.BilledID != invID {
			t.Fatalf("note %d BilledID = %+v, want %d", id, got.BilledID, invID)
		}
	}

	// nil invoiceID clears.
	if err := repo.MarkBilled(ctx, tid, nil, []int64{n1.ID}); err != nil {
		t.Fatalf("MarkBilled clear: %v", err)
	}
	got, _ := repo.Get(ctx, tid, n1.ID)
	if got == nil || got.BilledID != nil {
		t.Fatalf("note %d BilledID should be cleared, got %+v", n1.ID, got.BilledID)
	}
}
