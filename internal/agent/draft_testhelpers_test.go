package agent

// Shared seeding helpers for the divide-shift Smarts tests
// (smart_divide_shift_test.go). They reproduce the reference nursing-note
// fixture — participant "Tania Hangevelled", a FY26 catalogue carrying the two
// reference support items, and the four-day timesheet (referenceWeek) — as a
// tenant + participant + catalogue plus recorded note-only shifts.

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/google/uuid"
)

// The two support-item codes and prices from the reference invoice.
const (
	codeTransport  = "04_590_0125_6_1"
	priceTransport = 1.00
	codeSelfCare   = "01_011_0107_1_1"
	priceSelfCare  = 70.23
)

// noteDay is one day's worth of the timesheet: kilometres + hours.
type noteDay struct {
	date string
	km   float64
	hr   float64
}

// referenceWeek is the timesheet transcribed from the nursing note / PDF.
var referenceWeek = []noteDay{
	{"2026-06-09", 36, 7.0},
	{"2026-06-10", 12, 5.5},
	{"2026-06-11", 64, 7.0},
	{"2026-06-12", 38, 5.5},
}

func seedNoteTenant(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		Uuid: uuid.NewString(), Name: "Supreme care plus", Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return tn.ID
}

func seedNoteCatalogVersion(t *testing.T, conn *sql.DB, from, to string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	v, err := gen.New(conn).CreateCatalogVersion(context.Background(), gen.CreateCatalogVersionParams{
		Uuid: uuid.NewString(), Label: "NDIS FY26", EffectiveFrom: from,
		EffectiveTo: sql.NullString{String: to, Valid: true}, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed catalog version: %v", err)
	}
	return v.ID
}

// seedNoteItem adds a GST-free support item to a version, priced at `cap` in the
// national zone (the validator's default zone when no business profile exists).
func seedNoteItem(t *testing.T, conn *sql.DB, versionID int64, code, name string, cap float64) {
	t.Helper()
	q := gen.New(conn)
	si, err := q.CreateSupportItem(context.Background(), gen.CreateSupportItemParams{
		Uuid: uuid.NewString(), CatalogVersionID: versionID, Code: code, Name: name, GstFree: 1,
	})
	if err != nil {
		t.Fatalf("seed support item %s: %v", code, err)
	}
	if _, err := q.CreateSupportItemPrice(context.Background(), gen.CreateSupportItemPriceParams{
		SupportItemID: si.ID, Zone: "national", PriceCap: sql.NullFloat64{Float64: cap, Valid: true},
	}); err != nil {
		t.Fatalf("seed support item price %s: %v", code, err)
	}
}

// shiftToolsFixture opens a migrated temp DB and seeds the same tenant,
// participant and catalogue as the reference invoice fixture, returning the open
// connection plus the seeded tenant and participant ids.
func shiftToolsFixture(t *testing.T) (conn *sql.DB, tenantID, participantID int64) {
	t.Helper()
	c, err := appdb.Open(filepath.Join(t.TempDir(), "shifts.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := appdb.Migrate(c); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	tenantID = seedNoteTenant(t, c)
	ctx := reqctx.WithTenant(context.Background(), tenantID)

	p, err := participant.NewParticipants(c).Create(ctx, tenantID, participant.ParticipantInput{
		Name: "Tania Hangevelled", PlanStart: "2025-07-01", PlanEnd: "2026-06-30",
	})
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}

	verID := seedNoteCatalogVersion(t, c, "2025-07-01", "2026-06-30")
	seedNoteItem(t, c, verID, codeTransport, "Activity Based Transport", priceTransport)
	seedNoteItem(t, c, verID, codeSelfCare, "Assistance with self care - weekday daytime", priceSelfCare)

	return c, tenantID, p.ID
}

// seedReferenceShift inserts ONE nursing-note day as a recorded note-only shift
// (post-unification a shift carries no hours/km — those live on its line items)
// and returns the created shift. The note carries the activity narrative so the
// divide Smart has something to ground against.
func seedReferenceShift(t *testing.T, shifts *shift.Service, ctx context.Context, participantID int64, serviceDate string) *shift.Shift {
	t.Helper()
	sh, err := shifts.Create(ctx, shift.ShiftInput{
		ParticipantID: participantID,
		ServiceDate:   serviceDate,
		Note:          "Supported Tania with self care and community access.",
		Status:        "recorded",
	})
	if err != nil {
		t.Fatalf("seed shift %s: %v", serviceDate, err)
	}
	return sh
}
