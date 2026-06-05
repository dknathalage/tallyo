package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

// recurringFixture spins up a migrated DB plus a recurring repo, seeding a
// business profile and a client so snapshot building and invoice generation can
// be asserted end to end.
type recurringFixture struct {
	conn     *sql.DB
	repo     *RecurringRepo
	clientID int64
}

func newRecurringFixture(t *testing.T) recurringFixture {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "recurring.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	if err := NewBusinessProfile(conn).Save(ctx, BusinessProfileInput{
		Name: "My Biz", Email: "biz@x.com", Phone: "555", Address: "1 St",
	}); err != nil {
		t.Fatalf("Save business: %v", err)
	}
	client, err := NewClients(conn).Create(ctx, ClientInput{
		Name: "Acme", Email: "acme@x.com", Phone: "111", Address: "2 Ave",
	})
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}
	return recurringFixture{conn: conn, repo: NewRecurring(conn), clientID: client.ID}
}

func sampleRecurringLines() []RecurringLine {
	return []RecurringLine{
		{Description: "A", Quantity: 2, Rate: 10, SortOrder: 0},
		{Description: "B", Quantity: 1, Rate: 5, SortOrder: 1},
	}
}

func (f recurringFixture) validInput() RecurringInput {
	return RecurringInput{
		ClientID:  &f.clientID,
		Name:      "Monthly retainer",
		Frequency: "monthly",
		NextDue:   "2026-07-01",
		LineItems: sampleRecurringLines(),
		TaxRate:   10,
		Notes:     "thanks",
		IsActive:  true,
	}
}

func TestNewRecurringPanicsOnNilDB(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil db")
		}
	}()
	_ = NewRecurring(nil)
}

func TestRecurringCreateAndGet(t *testing.T) {
	f := newRecurringFixture(t)
	ctx := context.Background()

	tpl, err := f.repo.Create(ctx, f.validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tpl.ID == 0 || tpl.UUID == "" {
		t.Fatalf("Create returned empty id/uuid: %+v", tpl)
	}
	if tpl.Name != "Monthly retainer" || tpl.Frequency != "monthly" {
		t.Fatalf("unexpected header: %+v", tpl)
	}
	if tpl.ClientID == nil || *tpl.ClientID != f.clientID {
		t.Fatalf("ClientID = %v, want %d", tpl.ClientID, f.clientID)
	}
	if tpl.ClientName != "Acme" {
		t.Fatalf("ClientName = %q, want Acme", tpl.ClientName)
	}
	if len(tpl.LineItems) != 2 {
		t.Fatalf("LineItems len = %d, want 2", len(tpl.LineItems))
	}
	if tpl.LineItems[0].Description != "A" || tpl.LineItems[0].Quantity != 2 {
		t.Fatalf("line 0 = %+v", tpl.LineItems[0])
	}
	if !tpl.IsActive {
		t.Fatal("IsActive = false, want true")
	}

	got, err := f.repo.Get(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ID != tpl.ID || len(got.LineItems) != 2 {
		t.Fatalf("Get mismatch: %+v", got)
	}

	var count int
	if err := f.conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='recurring_template' AND action='create' AND entity_id=?",
		tpl.ID).Scan(&count); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit create count = %d, want 1", count)
	}
}

func TestRecurringGetMissing(t *testing.T) {
	f := newRecurringFixture(t)
	got, err := f.repo.Get(context.Background(), 9999)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatalf("Get missing = %+v, want nil", got)
	}
}

func TestRecurringCreateValidation(t *testing.T) {
	f := newRecurringFixture(t)
	ctx := context.Background()
	zero := int64(0)

	cases := []struct {
		name string
		in   RecurringInput
	}{
		{"empty name", func() RecurringInput { in := f.validInput(); in.Name = ""; return in }()},
		{"nil client", func() RecurringInput { in := f.validInput(); in.ClientID = nil; return in }()},
		{"zero client", func() RecurringInput { in := f.validInput(); in.ClientID = &zero; return in }()},
		{"bad frequency", func() RecurringInput { in := f.validInput(); in.Frequency = "daily"; return in }()},
		{"empty next due", func() RecurringInput { in := f.validInput(); in.NextDue = ""; return in }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.repo.Create(ctx, tc.in); err == nil {
				t.Fatalf("Create(%s) expected error", tc.name)
			}
		})
	}
}

func TestRecurringListAndListActive(t *testing.T) {
	f := newRecurringFixture(t)
	ctx := context.Background()

	active := f.validInput()
	active.NextDue = "2026-07-01"
	if _, err := f.repo.Create(ctx, active); err != nil {
		t.Fatalf("Create active: %v", err)
	}
	inactive := f.validInput()
	inactive.Name = "Paused"
	inactive.IsActive = false
	inactive.NextDue = "2026-08-01"
	if _, err := f.repo.Create(ctx, inactive); err != nil {
		t.Fatalf("Create inactive: %v", err)
	}

	all, err := f.repo.List(ctx, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List(all) len = %d, want 2", len(all))
	}
	onlyActive, err := f.repo.List(ctx, true)
	if err != nil {
		t.Fatalf("List active: %v", err)
	}
	if len(onlyActive) != 1 || onlyActive[0].Name != "Monthly retainer" {
		t.Fatalf("List(active) = %+v, want 1 active", onlyActive)
	}
}

func TestRecurringUpdate(t *testing.T) {
	f := newRecurringFixture(t)
	ctx := context.Background()

	tpl, err := f.repo.Create(ctx, f.validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	in := f.validInput()
	in.Name = "Renamed"
	in.IsActive = false
	in.Frequency = "weekly"
	updated, err := f.repo.Update(ctx, tpl.ID, in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil || updated.Name != "Renamed" || updated.IsActive || updated.Frequency != "weekly" {
		t.Fatalf("Update result = %+v", updated)
	}

	missing, err := f.repo.Update(ctx, 9999, f.validInput())
	if err != nil {
		t.Fatalf("Update missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Update missing = %+v, want nil", missing)
	}
}

func TestRecurringDelete(t *testing.T) {
	f := newRecurringFixture(t)
	ctx := context.Background()

	tpl, err := f.repo.Create(ctx, f.validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := f.repo.Delete(ctx, tpl.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := f.repo.Get(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("Get after delete = %+v, want nil", got)
	}
	var count int
	if err := f.conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='recurring_template' AND action='delete' AND entity_id=?",
		tpl.ID).Scan(&count); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit delete count = %d, want 1", count)
	}
}

func TestAdvanceDate(t *testing.T) {
	cases := []struct {
		freq, in, want string
	}{
		{"weekly", "2026-06-05", "2026-06-12"},
		{"monthly", "2026-06-05", "2026-07-05"},
		{"quarterly", "2026-06-05", "2026-09-05"},
	}
	r := &RecurringRepo{}
	for _, tc := range cases {
		t.Run(tc.freq, func(t *testing.T) {
			got, err := r.AdvanceDate(tc.in, tc.freq)
			if err != nil {
				t.Fatalf("AdvanceDate: %v", err)
			}
			if got != tc.want {
				t.Fatalf("AdvanceDate(%s,%s) = %q, want %q", tc.in, tc.freq, got, tc.want)
			}
		})
	}
	if _, err := r.AdvanceDate("2026-06-05", "yearly"); err == nil {
		t.Fatal("AdvanceDate(unknown freq) expected error")
	}
	if _, err := r.AdvanceDate("not-a-date", "weekly"); err == nil {
		t.Fatal("AdvanceDate(bad date) expected error")
	}
}

func TestGenerateOne(t *testing.T) {
	f := newRecurringFixture(t)
	ctx := context.Background()

	in := f.validInput()
	in.NextDue = "2026-06-05"
	in.Frequency = "monthly"
	tpl, err := f.repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	inv, err := f.repo.GenerateOne(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("GenerateOne: %v", err)
	}
	if inv == nil {
		t.Fatal("GenerateOne returned nil invoice")
	}
	if inv.Status != "draft" {
		t.Fatalf("Status = %q, want draft", inv.Status)
	}
	if inv.InvoiceNumber == "" {
		t.Fatalf("InvoiceNumber empty")
	}
	if inv.Total != 27.5 {
		t.Fatalf("Total = %v, want 27.5", inv.Total)
	}
	if len(inv.LineItems) != 2 {
		t.Fatalf("LineItems len = %d, want 2", len(inv.LineItems))
	}

	got, err := f.repo.Get(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("Get template: %v", err)
	}
	if got.NextDue != "2026-07-05" {
		t.Fatalf("NextDue = %q, want 2026-07-05 (advanced)", got.NextDue)
	}

	// missing template -> (nil, nil)
	missing, err := f.repo.GenerateOne(ctx, 9999)
	if err != nil {
		t.Fatalf("GenerateOne missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("GenerateOne missing = %+v, want nil", missing)
	}
}

func TestGenerateDueIdempotent(t *testing.T) {
	f := newRecurringFixture(t)
	ctx := context.Background()

	today := time.Now().UTC()
	yesterday := today.AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := today.AddDate(0, 0, 1).Format("2006-01-02")

	due := f.validInput()
	due.Name = "Due"
	due.Frequency = "weekly"
	due.NextDue = yesterday
	dueTpl, err := f.repo.Create(ctx, due)
	if err != nil {
		t.Fatalf("Create due: %v", err)
	}

	future := f.validInput()
	future.Name = "Future"
	future.NextDue = tomorrow
	if _, err := f.repo.Create(ctx, future); err != nil {
		t.Fatalf("Create future: %v", err)
	}

	gen1, err := f.repo.GenerateDue(ctx)
	if err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	if len(gen1) != 1 {
		t.Fatalf("first GenerateDue produced %d, want 1", len(gen1))
	}
	if gen1[0].TemplateID != dueTpl.ID {
		t.Fatalf("generated template = %d, want %d", gen1[0].TemplateID, dueTpl.ID)
	}

	// the generated invoice exists
	inv, err := NewInvoices(f.conn).Get(ctx, gen1[0].InvoiceID)
	if err != nil {
		t.Fatalf("Get invoice: %v", err)
	}
	if inv == nil || inv.InvoiceNumber != gen1[0].InvoiceNumber {
		t.Fatalf("generated invoice not found / mismatch: %+v", inv)
	}

	// next_due advanced beyond today
	advanced, err := f.repo.Get(ctx, dueTpl.ID)
	if err != nil {
		t.Fatalf("Get advanced: %v", err)
	}
	if advanced.NextDue <= today.Format("2006-01-02") {
		t.Fatalf("NextDue = %q not advanced past today", advanced.NextDue)
	}

	// re-running the sweep is a no-op (idempotent)
	gen2, err := f.repo.GenerateDue(ctx)
	if err != nil {
		t.Fatalf("second GenerateDue: %v", err)
	}
	if len(gen2) != 0 {
		t.Fatalf("second GenerateDue produced %d, want 0 (idempotent)", len(gen2))
	}
}
