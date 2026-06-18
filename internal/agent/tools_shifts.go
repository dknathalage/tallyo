package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// candidateView is a curated catalogue suggestion attached to a shift, derived
// from the shift's structured measures and resolved for its service date. The
// model should prefer picking a code from these over a free-form search.
type candidateView struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Unit     string   `json:"unit"`
	PriceCap *float64 `json:"priceCap"`
	GstFree  bool     `json:"gstFree"`
}

// listParticipantShiftsInput is the parsed input for the list_participant_shifts
// tool. From/To are an optional inclusive service-date window (YYYY-MM-DD); both
// empty returns every shift for the participant.
type listParticipantShiftsInput struct {
	ParticipantID int64  `json:"participantId"`
	From          string `json:"from"`
	To            string `json:"to"`
}

// listParticipantShiftsSchema is the model-facing JSON schema for the tool.
const listParticipantShiftsSchema = `{
  "type": "object",
  "properties": {
    "participantId": { "type": "integer", "description": "Id of the participant whose shifts to list." },
    "from": { "type": "string", "description": "Optional inclusive start of the service-date range (YYYY-MM-DD)." },
    "to": { "type": "string", "description": "Optional inclusive end of the service-date range (YYYY-MM-DD)." }
  },
  "required": ["participantId"],
  "additionalProperties": false
}`

// shiftView is the sanitised, model-facing projection of a shift. The free-text
// Note is fenced via wrapUntrusted so the model treats it as data, never as
// instructions; the remaining fields are structured values passed through as-is.
// Candidates is a small curated set of likely NDIS codes derived from the
// shift's measures (hours → self-care, km → transport), omitted when no
// catalogue is wired in.
type shiftView struct {
	ID            int64           `json:"id"`
	ParticipantID int64           `json:"participantId"`
	ServiceDate   string          `json:"serviceDate"`
	Hours         float64         `json:"hours"`
	Km            float64         `json:"km"`
	Note          string          `json:"note"`
	Tags          []string        `json:"tags"`
	Status        string          `json:"status"`
	InvoiceID     *int64          `json:"invoiceId"`
	Candidates    []candidateView `json:"candidates,omitempty"`
}

// NewListParticipantShiftsTool returns a read tool that lists a participant's
// recorded shifts within an optional service-date range. Call this when drafting
// an invoice from shifts. Shift notes are free text authored by users, so each
// is returned fenced as untrusted content (the model must treat the note as
// data, not instructions).
func NewListParticipantShiftsTool(shifts *service.ShiftService) Tool {
	return newListParticipantShiftsTool(shifts, nil)
}

// NewListParticipantShiftsToolWithCatalog returns the same list_participant_shifts
// tool but, for each shift, attaches a small curated set of candidate NDIS codes
// derived from the shift's measures (hours → self-care, km → transport) and
// resolved for the shift's service date. This lets the model pick the code from a
// short list instead of free-form searching, cutting search_catalogue round-trips
// and code-mapping errors.
func NewListParticipantShiftsToolWithCatalog(shifts *service.ShiftService, cat *catalog.Service) Tool {
	return newListParticipantShiftsTool(shifts, cat)
}

// newListParticipantShiftsTool builds the list_participant_shifts tool. When cat
// is non-nil it enriches each shift with candidate catalogue codes; cat == nil
// yields the plain (no-candidates) behaviour, keeping the original constructor
// unchanged for existing callers and tests.
func newListParticipantShiftsTool(shifts *service.ShiftService, cat *catalog.Service) Tool {
	return Tool{
		Name:        "list_participant_shifts",
		Description: "List a participant's recorded shifts within an optional date range. Call this when drafting an invoice from shifts.",
		Risk:        RiskRead,
		Render:      "table",
		Schema:      json.RawMessage(listParticipantShiftsSchema),
		Handler: func(ctx context.Context, input json.RawMessage) (Result, error) {
			var in listParticipantShiftsInput
			if err := json.Unmarshal(input, &in); err != nil {
				return Result{}, fmt.Errorf("list_participant_shifts: invalid input: %w", err)
			}
			if in.ParticipantID <= 0 {
				return Result{}, fmt.Errorf("list_participant_shifts: participantId must be a positive integer")
			}
			records, err := shifts.ListParticipant(ctx, in.ParticipantID, in.From, in.To)
			if err != nil {
				return Result{}, fmt.Errorf("list_participant_shifts: %w", err)
			}
			rows := make([]shiftView, 0, len(records))
			for i := range records { // bounded by len(records)
				sh := records[i]
				rows = append(rows, shiftView{
					ID:            sh.ID,
					ParticipantID: sh.ParticipantID,
					ServiceDate:   sh.ServiceDate,
					Hours:         sh.Hours,
					Km:            sh.Km,
					Note:          wrapUntrusted("shift-note", sh.Note),
					Tags:          sh.Tags,
					Status:        sh.Status,
					InvoiceID:     sh.InvoiceID,
					Candidates:    shiftCandidates(ctx, cat, sh),
				})
			}
			return Result{JSON: rows, Render: "table"}, nil
		},
	}
}

// shiftCandidates resolves a small deduped set of catalogue suggestions for a
// shift from its structured measures (hours → self-care, km → transport), for
// the shift's service date. It is best-effort: cat == nil or any lookup error
// yields nil (no candidates) and never fails the read.
func shiftCandidates(ctx context.Context, cat *catalog.Service, sh *repository.Shift) []candidateView {
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

// searchCatalogueInput is the parsed input for the search_catalogue tool. Zone
// is optional and defaults to the national zone when empty.
type searchCatalogueInput struct {
	Query       string `json:"query"`
	ServiceDate string `json:"serviceDate"`
	Zone        string `json:"zone"`
}

// searchCatalogueSchema is the model-facing JSON schema for the tool.
const searchCatalogueSchema = `{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Keyword or NDIS support item code to search for." },
    "serviceDate": { "type": "string", "description": "Service date the catalogue must be effective on (YYYY-MM-DD)." },
    "zone": { "type": "string", "description": "Optional pricing zone; defaults to national." }
  },
  "required": ["query", "serviceDate"],
  "additionalProperties": false
}`

// NewSearchCatalogueTool returns a read tool that searches the NDIS support-item
// catalogue effective on a service date by code or keyword, returning each
// item's code, name, unit, GST-free flag and the national price cap. Call this
// to find the correct NDIS code and rate for an activity before creating an
// invoice.
func NewSearchCatalogueTool(cat *catalog.Service) Tool {
	return Tool{
		Name:        "search_catalogue",
		Description: "Search the NDIS support-item catalogue effective on a service date by code or keyword, returning each item's code, name, unit, GST-free flag and the national price cap. Call this to find the correct NDIS code and rate for an activity before creating an invoice.",
		Risk:        RiskRead,
		Render:      "table",
		Schema:      json.RawMessage(searchCatalogueSchema),
		Handler: func(ctx context.Context, input json.RawMessage) (Result, error) {
			var in searchCatalogueInput
			if err := json.Unmarshal(input, &in); err != nil {
				return Result{}, fmt.Errorf("search_catalogue: invalid input: %w", err)
			}
			if in.ServiceDate == "" {
				return Result{}, fmt.Errorf("search_catalogue: serviceDate is required")
			}
			matches, err := cat.SearchForDate(ctx, in.Query, in.ServiceDate, in.Zone, 25)
			if err != nil {
				return Result{}, fmt.Errorf("search_catalogue: %w", err)
			}
			return Result{JSON: matches, Render: "table"}, nil
		},
	}
}
