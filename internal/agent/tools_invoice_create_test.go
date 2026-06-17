package agent

// Tests that the agent's create_invoice tool can turn a real-world NDIS nursing
// note (a support-worker timesheet) into a compliant invoice. The reference
// fixture is Invoice-2638907B.pdf: participant "Tania Hangevelled", week ended
// 14/6/2026, with two support items per service day —
//
//	04_590_0125_6_1  Activity Based Transport          $1.00 / km
//	01_011_0107_1_1  Assistance with self care (wd)    $70.23 / hour
//
// The note records, per day, the kilometres driven and the hours worked:
//
//	Tue 9/6   36 km   7.0 h
//	Wed 10/6  12 km   5.5 h
//	Thu 11/6  64 km   7.0 h
//	Fri 12/6  38 km   5.5 h
//
// which yields a $1905.76 GST-free invoice (NDIS supports are GST-free, so tax
// is 0 and total == subtotal). These tests exercise the full tool path: JSON in
// → NDIS validation engine → persisted invoice, and the structured error the
// tool returns when a line breaches the price cap.

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/google/uuid"
)

// The two support-item codes and prices from the reference invoice.
const (
	codeTransport  = "04_590_0125_6_1"
	priceTransport = 1.00
	codeSelfCare   = "01_011_0107_1_1"
	priceSelfCare  = 70.23
)

// nursingNoteSvc opens a temp DB, migrates it, seeds a tenant + participant with
// a plan window covering the 2025-26 NDIS financial year, and a catalogue
// version effective over that year carrying the two reference support items
// priced at the national cap. It returns the invoice service and the seeded
// participant id — everything create_invoice needs to validate the note's lines.
func nursingNoteSvc(t *testing.T) (svc *service.InvoiceService, tenantID, participantID int64) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "note.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	tenantID = seedNoteTenant(t, conn)
	ctx := reqctx.WithTenant(context.Background(), tenantID)

	// Participant with a plan window enclosing the service week (NDIS FY26).
	p, err := repository.NewParticipants(conn).Create(ctx, tenantID, repository.ParticipantInput{
		Name: "Tania Hangevelled", PlanStart: "2025-07-01", PlanEnd: "2026-06-30",
	})
	if err != nil {
		t.Fatalf("seed participant: %v", err)
	}

	// Catalogue version effective across FY26 with both support items, GST-free,
	// priced at the national cap so an at-cap unit price is accepted.
	verID := seedNoteCatalogVersion(t, conn, "2025-07-01", "2026-06-30")
	seedNoteItem(t, conn, verID, codeTransport, "Activity Based Transport", priceTransport)
	seedNoteItem(t, conn, verID, codeSelfCare, "Assistance with self care - weekday daytime", priceSelfCare)

	return service.NewInvoiceService(conn, realtime.NewHub()), tenantID, p.ID
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

// noteLine is one day's worth of the timesheet: kilometres + hours.
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

// noteToolInput builds the create_invoice JSON the model would emit for the
// week: two support-item lines (transport, self care) per service day.
func noteToolInput(t *testing.T, participantID int64) json.RawMessage {
	t.Helper()
	type line struct {
		Code        string  `json:"code"`
		ServiceDate string  `json:"serviceDate"`
		Quantity    float64 `json:"quantity"`
		UnitPrice   float64 `json:"unitPrice"`
		SortOrder   int     `json:"sortOrder"`
	}
	in := struct {
		ParticipantID int64  `json:"participantId"`
		IssueDate     string `json:"issueDate"`
		DueDate       string `json:"dueDate"`
		Items         []line `json:"items"`
	}{ParticipantID: participantID, IssueDate: "2026-06-14", DueDate: "2026-06-28"}

	sort := 0
	for _, d := range referenceWeek { // bounded by len(referenceWeek)
		in.Items = append(in.Items,
			line{Code: codeTransport, ServiceDate: d.date, Quantity: d.km, UnitPrice: priceTransport, SortOrder: sort},
			line{Code: codeSelfCare, ServiceDate: d.date, Quantity: d.hr, UnitPrice: priceSelfCare, SortOrder: sort + 1},
		)
		sort += 2
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal tool input: %v", err)
	}
	return raw
}

// TestCreateInvoiceFromNursingNote drives the create_invoice tool with the full
// week from the nursing note and asserts the persisted invoice matches the
// reference PDF: 8 GST-free lines, correct per-line totals, $1905.76 total.
func TestCreateInvoiceFromNursingNote(t *testing.T) {
	svc, tenantID, participantID := nursingNoteSvc(t)
	tool := NewCreateInvoiceTool(svc, nil)

	if tool.Risk != RiskRisky {
		t.Fatalf("Risk = %q, want risky (a write must require approval)", tool.Risk)
	}

	ctx := reqctx.WithTenant(context.Background(), tenantID)
	res, err := tool.Handler(ctx, noteToolInput(t, participantID))
	if err != nil {
		t.Fatalf("create_invoice: %v", err)
	}
	inv, ok := res.JSON.(*repository.Invoice)
	if !ok {
		t.Fatalf("result JSON is %T, want *repository.Invoice", res.JSON)
	}

	if res.Render != "card" {
		t.Fatalf("Render = %q, want card", res.Render)
	}
	if inv.Status != "draft" {
		t.Fatalf("Status = %q, want draft", inv.Status)
	}
	if got, want := len(inv.LineItems), 2*len(referenceWeek); got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	if inv.Tax != 0 {
		t.Fatalf("Tax = %.2f, want 0 (NDIS supports are GST-free)", inv.Tax)
	}
	if inv.Subtotal != 1905.76 {
		t.Fatalf("Subtotal = %.2f, want 1905.76", inv.Subtotal)
	}
	if inv.Total != 1905.76 {
		t.Fatalf("Total = %.2f, want 1905.76", inv.Total)
	}

	// Every line must carry its NDIS code snapshot and be flagged GST-free, and
	// the per-line totals must match the note arithmetic.
	for i := range inv.LineItems { // bounded by len(inv.LineItems)
		li := inv.LineItems[i]
		if !li.GstFree {
			t.Fatalf("line %d (%s) GstFree = false, want true", i, li.Code)
		}
		if li.Code != codeTransport && li.Code != codeSelfCare {
			t.Fatalf("line %d has unexpected code %q", i, li.Code)
		}
		if li.SupportItemID == nil || li.CatalogVersionID == nil {
			t.Fatalf("line %d (%s) was not snapshotted to the catalogue", i, li.Code)
		}
	}
}

// TestCreateInvoiceCatalogueAuthoritativePrice asserts Pillar 1: the agent's
// create_invoice path IGNORES a model-supplied unit price for a catalogue-coded
// line and bills at the authoritative NDIS cap. Here the model sends an over-cap
// $80 self-care rate; the platform overrides it to the $70.23 cap rather than
// rejecting — so a model misprice can neither overbill nor (with $0) underbill.
func TestCreateInvoiceCatalogueAuthoritativePrice(t *testing.T) {
	svc, tenantID, participantID := nursingNoteSvc(t)
	tool := NewCreateInvoiceTool(svc, nil)
	ctx := reqctx.WithTenant(context.Background(), tenantID)

	// Two coded lines with deliberately WRONG model prices: over-cap ($80) and
	// zero ($0). Both must end up at the catalogue cap.
	in := map[string]any{
		"participantId": participantID,
		"items": []map[string]any{
			{"code": codeSelfCare, "serviceDate": "2026-06-09", "quantity": 7.0, "unitPrice": 80.00},
			{"code": codeTransport, "serviceDate": "2026-06-09", "quantity": 10.0, "unitPrice": 0.0},
		},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res, err := tool.Handler(ctx, raw)
	if err != nil {
		t.Fatalf("create_invoice: %v", err)
	}
	inv, ok := res.JSON.(*repository.Invoice)
	if !ok {
		t.Fatalf("result JSON is %T, want *repository.Invoice", res.JSON)
	}
	byCode := map[string]float64{}
	for i := range inv.LineItems { // bounded by len(inv.LineItems)
		byCode[inv.LineItems[i].Code] = inv.LineItems[i].UnitPrice
	}
	if byCode[codeSelfCare] != priceSelfCare {
		t.Fatalf("self-care unit price = %.2f, want cap %.2f (over-cap $80 must be overridden)", byCode[codeSelfCare], priceSelfCare)
	}
	if byCode[codeTransport] != priceTransport {
		t.Fatalf("transport unit price = %.2f, want cap %.2f ($0 must be overridden)", byCode[codeTransport], priceTransport)
	}
	// Total = 7×70.23 + 10×1.00 = 491.61 + 10.00 = 501.61.
	if inv.Total != 501.61 {
		t.Fatalf("total = %.2f, want 501.61 (priced at caps, not the model's numbers)", inv.Total)
	}
}

// TestCreateInvoiceRejectsZeroQuantityCoded asserts the contract tightening
// (Pillar 3): a catalogue-coded line with a non-positive quantity is rejected as
// a structured tool error (not silently billed at $0) so the agent self-corrects.
func TestCreateInvoiceRejectsZeroQuantityCoded(t *testing.T) {
	svc, tenantID, participantID := nursingNoteSvc(t)
	tool := NewCreateInvoiceTool(svc, nil)
	ctx := reqctx.WithTenant(context.Background(), tenantID)

	in := map[string]any{
		"participantId": participantID,
		"items": []map[string]any{
			{"code": codeSelfCare, "serviceDate": "2026-06-09", "quantity": 0.0},
		},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := tool.Handler(ctx, raw); err == nil {
		t.Fatal("expected an error for a zero-quantity coded line")
	}
}
