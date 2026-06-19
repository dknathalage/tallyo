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
		key := shiftDedupKey(d.ServiceDate, d.StartTime, d.EndTime, d.Hours, d.Km)
		if _, dup := seen[key]; dup {
			continue // already recorded (or an in-batch duplicate)
		}
		seen[key] = struct{}{}
		sh, e := s.shifts.Create(ctx, shift.ShiftInput{
			ParticipantID: participantID,
			ServiceDate:   d.ServiceDate,
			StartTime:     d.StartTime,
			EndTime:       d.EndTime,
			Hours:         d.Hours,
			Km:            d.Km,
			Note:          d.Note,
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
		sh := existing[i]
		keys[shiftDedupKey(sh.ServiceDate, sh.StartTime, sh.EndTime, sh.Hours, sh.Km)] = struct{}{}
	}
	return keys, nil
}

// shiftDedupKey is the natural identity of a shift for import idempotency:
// service date, start/end time (as written) and the billable hours/km. The free
// note is deliberately excluded — model re-extractions can reword it, which would
// defeat dedup — while date+times+quantities pin the same delivered shift.
func shiftDedupKey(serviceDate, startTime, endTime string, hours, km float64) string {
	return fmt.Sprintf("%s\x1f%s\x1f%s\x1f%.3f\x1f%.3f", serviceDate, startTime, endTime, hours, km)
}
