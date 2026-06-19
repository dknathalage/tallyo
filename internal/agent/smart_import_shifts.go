package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/dknathalage/tallyo/internal/shift"
)

// ImportShifts turns a free-text timesheet into recorded shifts for one
// participant. It forces a single structured-extraction model call
// (ExtractShifts), then persists one recorded shift per extracted day via the
// shift service. The participant is taken from the caller (resolution by name is
// intentionally out of scope here). Returns the created shifts.
func (s *Smarts) ImportShifts(ctx context.Context, participantID int64, text string) ([]*shift.Shift, error) {
	if participantID <= 0 {
		return nil, fmt.Errorf("import shifts: participantId is required")
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("import shifts: text is required")
	}

	drafts, err := ExtractShifts(ctx, s.client, s.cfg.Model, s.cfg.EffortFor(), text)
	if err != nil {
		return nil, fmt.Errorf("import shifts: extract: %w", err)
	}

	// Idempotency: skip drafts that match a shift already recorded for this
	// participant. Re-importing the same timesheet (or one that overlaps a prior
	// import) must not create duplicates. The set also dedups within this batch.
	seen, err := s.existingShiftKeys(ctx, participantID, drafts)
	if err != nil {
		return nil, fmt.Errorf("import shifts: load existing: %w", err)
	}

	created := make([]*shift.Shift, 0, len(drafts))
	for i := range drafts { // bounded by len(drafts)
		d := drafts[i]
		key := shiftDedupKey(d.ServiceDate)
		if _, dup := seen[key]; dup {
			continue // already recorded (or an in-batch duplicate)
		}
		seen[key] = struct{}{}
		sh, e := s.shifts.Create(ctx, shift.ShiftInput{
			ParticipantID: participantID,
			ServiceDate:   d.ServiceDate,
			Note:          composeNote(d.Note, d.Hours, d.Km),
			Status:        "recorded",
		})
		if e != nil {
			return nil, fmt.Errorf("import shifts: create: %w", e)
		}
		created = append(created, sh)
	}
	return created, nil
}

// existingShiftKeys returns the dedup-key set of the participant's already
// recorded shifts that fall within the drafts' service-date span. It queries only
// that window (the min..max draft date) rather than every shift, then keys each
// existing row the same way a draft is keyed so re-imports are detected.
func (s *Smarts) existingShiftKeys(ctx context.Context, participantID int64, drafts []ShiftDraft) (map[string]struct{}, error) {
	keys := make(map[string]struct{}, len(drafts))
	if len(drafts) == 0 {
		return keys, nil
	}
	from, to := drafts[0].ServiceDate, drafts[0].ServiceDate
	for i := range drafts { // bounded by len(drafts); ISO dates sort lexicographically
		if d := drafts[i].ServiceDate; d < from {
			from = d
		} else if d > to {
			to = d
		}
	}
	existing, err := s.shifts.ListParticipant(ctx, participantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list existing shifts: %w", err)
	}
	for i := range existing { // bounded by len(existing)
		keys[shiftDedupKey(existing[i].ServiceDate)] = struct{}{}
	}
	return keys, nil
}

// composeNote folds the extracted hours/km quantities into the shift note so a
// later DivideShift (or the user) can recover them — post-unification a shift
// carries no hours/km columns. A summary like "[support 7.0h; travel 36km]" is
// appended only for the non-zero quantities; with neither, the note is returned
// unchanged.
func composeNote(note string, hours, km float64) string {
	note = strings.TrimSpace(note)
	parts := make([]string, 0, 2)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("support %.1fh", hours))
	}
	if km > 0 {
		parts = append(parts, fmt.Sprintf("travel %.0fkm", km))
	}
	if len(parts) == 0 {
		return note
	}
	summary := "[" + strings.Join(parts, "; ") + "]"
	if note == "" {
		return summary
	}
	return note + " " + summary
}

// shiftDedupKey is the natural identity of a shift for import idempotency:
// post-unification a shift carries no times/quantities (those moved onto line
// items), so the service date pins the same delivered shift. The free note is
// deliberately excluded — model re-extractions can reword it, which would defeat
// dedup.
func shiftDedupKey(serviceDate string) string {
	return serviceDate
}
