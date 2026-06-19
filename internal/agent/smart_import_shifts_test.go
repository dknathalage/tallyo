package agent

// Tests for Smarts.ImportShifts: free-text timesheet → recorded shifts via a
// forced emit_shifts extraction followed by a deterministic, idempotent create
// step. The model call is driven by a scripted llm.Fake (an emit_shifts tool_use
// carrying the drafts); persistence is driven by a stub ShiftWorker that records
// created shifts, returns a fixed set of "existing" shifts, and can be told to
// fail on Create.

import (
	"context"
	"fmt"
	"testing"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/shift"
)

// stubShiftWorker is an in-memory ShiftWorker for ImportShifts tests. existing is
// returned (filtered to nothing — the whole slice) by ListParticipant; created
// records every Create; createErr makes Create fail.
type stubShiftWorker struct {
	existing  []*shift.Shift
	created   []*shift.Shift
	createErr error
	nextID    int64
}

func (s *stubShiftWorker) ListParticipant(_ context.Context, _ int64, _, _ string) ([]*shift.Shift, error) {
	return s.existing, nil
}

func (s *stubShiftWorker) MarkDrafted(_ context.Context, _ int64, _ []int64) error { return nil }

func (s *stubShiftWorker) Create(_ context.Context, in shift.ShiftInput) (*shift.Shift, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.nextID++
	sh := &shift.Shift{
		ID:            s.nextID,
		ParticipantID: in.ParticipantID,
		ServiceDate:   in.ServiceDate,
		StartTime:     in.StartTime,
		EndTime:       in.EndTime,
		Hours:         in.Hours,
		Km:            in.Km,
		Note:          in.Note,
		Status:        in.Status,
	}
	s.created = append(s.created, sh)
	return sh, nil
}

// stubInvoiceCreator / stubCatalogueSearcher satisfy NewSmarts' non-nil
// requirement; ImportShifts never calls them.
type stubInvoiceCreator struct{}

func (stubInvoiceCreator) CreateWithCatalogPricing(_ context.Context, _ invoice.InvoiceInput, _ []billing.LineItemInput) (*invoice.Invoice, error) {
	return nil, fmt.Errorf("not used")
}

type stubCatalogueSearcher struct{}

func (stubCatalogueSearcher) SearchForDate(_ context.Context, _, _, _ string, _ int) ([]*catalog.CatalogMatch, error) {
	return nil, fmt.Errorf("not used")
}

// newImportSmarts builds a Smarts whose model returns the given drafts from a
// forced emit_shifts call, backed by the supplied stub worker.
func newImportSmarts(t *testing.T, drafts []ShiftDraft, worker *stubShiftWorker) *Smarts {
	t.Helper()
	fake := llm.NewFake(emitShiftsResponse(t, drafts))
	cfg := Config{APIKey: "sk-x", Model: "model-x", Effort: "high"}.WithDefaults()
	return NewSmarts(cfg, fake, stubInvoiceCreator{}, worker, stubCatalogueSearcher{})
}

// TestImportShiftsReimportIdempotent: a draft matching an already-recorded shift
// is skipped, so a re-import creates nothing.
func TestImportShiftsReimportIdempotent(t *testing.T) {
	drafts := []ShiftDraft{
		{ParticipantName: "Tania", ServiceDate: "2026-06-09", StartTime: "9am", EndTime: "4pm", Hours: 7.0, Km: 36},
	}
	worker := &stubShiftWorker{existing: []*shift.Shift{
		{ID: 1, ServiceDate: "2026-06-09", StartTime: "9am", EndTime: "4pm", Hours: 7.0, Km: 36},
	}}
	s := newImportSmarts(t, drafts, worker)

	got, err := s.ImportShifts(context.Background(), 42, "some timesheet text")
	if err != nil {
		t.Fatalf("ImportShifts: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("re-import must create nothing, created %d", len(got))
	}
	if len(worker.created) != 0 {
		t.Fatalf("Create called %d times, want 0", len(worker.created))
	}
}

// TestImportShiftsInBatchDuplicate: two identical drafts in one call create only
// one shift (in-batch dedup).
func TestImportShiftsInBatchDuplicate(t *testing.T) {
	d := ShiftDraft{ParticipantName: "Tania", ServiceDate: "2026-06-10", StartTime: "9am", EndTime: "2.30pm", Hours: 5.5, Km: 12}
	drafts := []ShiftDraft{d, d}
	worker := &stubShiftWorker{}
	s := newImportSmarts(t, drafts, worker)

	got, err := s.ImportShifts(context.Background(), 42, "timesheet text")
	if err != nil {
		t.Fatalf("ImportShifts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("two identical drafts must create 1 shift, created %d", len(got))
	}
	if len(worker.created) != 1 {
		t.Fatalf("Create called %d times, want 1", len(worker.created))
	}
}

// TestImportShiftsCreateErrorPropagates: a Create failure surfaces as an error.
func TestImportShiftsCreateErrorPropagates(t *testing.T) {
	drafts := []ShiftDraft{
		{ParticipantName: "Tania", ServiceDate: "2026-06-11", Hours: 7.0, Km: 64},
	}
	worker := &stubShiftWorker{createErr: fmt.Errorf("db down")}
	s := newImportSmarts(t, drafts, worker)

	if _, err := s.ImportShifts(context.Background(), 42, "timesheet text"); err == nil {
		t.Fatal("expected ImportShifts to surface a Create error")
	}
}
