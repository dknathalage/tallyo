package agent

// Tests for the catalogue-aware list_participant_notes variant
// (NewListParticipantNotesToolWithCatalog): each returned note carries a small
// curated set of candidate NDIS codes derived from its structured activity tag
// and resolved for the note's service date, so the model can pick a code
// without a free-form search_catalogue round-trip. The plain constructor must
// stay candidate-free (back-compat).

import (
	"context"
	"testing"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

// findCandidate returns the first candidate with the given code, or nil.
func findCandidate(cands []candidateView, code string) *candidateView {
	for i := range cands { // bounded by len(cands)
		if cands[i].Code == code {
			return &cands[i]
		}
	}
	return nil
}

// TestListParticipantNotesAttachesCandidates asserts the catalogue-aware tool
// attaches the correct candidate codes (transport + self-care, since every
// reference day has both km and hours) with their price caps to each note.
func TestListParticipantNotesAttachesCandidates(t *testing.T) {
	conn, tenantID, participantID := noteToolsFixture(t)
	notes := service.NewNoteService(conn, realtime.NewHub())
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	seedReferenceNotes(t, notes, ctx, participantID)

	cat := service.NewSupportCatalogService(conn)
	tool := NewListParticipantNotesToolWithCatalog(notes, cat)

	rows := runListNotes(t, tool, ctx, participantID, "2026-06-09", "2026-06-12")
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}

	for i := range rows { // bounded by len(rows)
		r := rows[i]
		if len(r.Candidates) == 0 {
			t.Fatalf("note %s (%d): no candidates attached", r.ServiceDate, r.ID)
		}

		// Every reference day has transportKm > 0 → transport candidate.
		tc := findCandidate(r.Candidates, codeTransport)
		if tc == nil {
			t.Fatalf("note %s: missing transport candidate %s; got %+v", r.ServiceDate, codeTransport, r.Candidates)
		}
		if tc.PriceCap == nil || *tc.PriceCap != priceTransport {
			t.Fatalf("note %s: transport priceCap = %v, want %.2f", r.ServiceDate, tc.PriceCap, priceTransport)
		}

		// Every reference day has supportHours > 0 → self-care candidate.
		sc := findCandidate(r.Candidates, codeSelfCare)
		if sc == nil {
			t.Fatalf("note %s: missing self-care candidate %s; got %+v", r.ServiceDate, codeSelfCare, r.Candidates)
		}
		if sc.PriceCap == nil || *sc.PriceCap != priceSelfCare {
			t.Fatalf("note %s: self-care priceCap = %v, want %.2f", r.ServiceDate, sc.PriceCap, priceSelfCare)
		}
	}
}

// TestListParticipantNotesPlainHasNoCandidates asserts the original constructor
// (no catalogue) still returns notes without any candidates (back-compat).
func TestListParticipantNotesPlainHasNoCandidates(t *testing.T) {
	conn, tenantID, participantID := noteToolsFixture(t)
	notes := service.NewNoteService(conn, realtime.NewHub())
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	seedReferenceNotes(t, notes, ctx, participantID)

	tool := NewListParticipantNotesTool(notes)
	rows := runListNotes(t, tool, ctx, participantID, "2026-06-09", "2026-06-12")
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	for i := range rows { // bounded by len(rows)
		if len(rows[i].Candidates) != 0 {
			t.Fatalf("note %s: plain tool attached candidates %+v, want none", rows[i].ServiceDate, rows[i].Candidates)
		}
	}
}
