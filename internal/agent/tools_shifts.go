package agent

import (
	"context"

	"github.com/dknathalage/tallyo/internal/shift"
)

// candidateView is a curated catalogue suggestion attached to a shift, derived
// from the shift's structured measures and resolved for its service date. The
// draft-invoice gather attaches these so the model picks a code from a short
// list instead of free-form searching.
type candidateView struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Unit     string   `json:"unit"`
	PriceCap *float64 `json:"priceCap"`
	GstFree  bool     `json:"gstFree"`
}

// shiftCandidates resolves a small deduped set of catalogue suggestions for a
// shift from its structured measures (hours → self-care, km → transport), for
// the shift's service date. It is best-effort: cat == nil or any lookup error
// yields nil (no candidates) and never fails the read.
func shiftCandidates(ctx context.Context, cat CatalogueSearcher, sh *shift.Shift) []candidateView {
	if cat == nil || sh == nil || sh.ServiceDate == "" {
		return nil
	}
	seen := make(map[string]struct{}, 4)
	out := make([]candidateView, 0, 4)
	add := func(query string) {
		matches, err := cat.SearchForDate(ctx, query, sh.ServiceDate, "", 3)
		if err != nil {
			return // best-effort: skip on error, never fail the read
		}
		for i := range matches { // bounded by len(matches) (≤3)
			m := matches[i]
			if m == nil {
				continue
			}
			if _, dup := seen[m.Code]; dup {
				continue
			}
			seen[m.Code] = struct{}{}
			out = append(out, candidateView{
				Code:     m.Code,
				Name:     m.Name,
				Unit:     m.Unit,
				PriceCap: m.PriceCap,
				GstFree:  m.GstFree,
			})
		}
	}
	if sh.Km > 0 {
		add("transport")
	}
	if sh.Hours > 0 {
		before := len(out)
		add("self-care")
		if len(out) == before { // nothing matched; try the spelt-out phrasing
			add("assistance with self")
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
