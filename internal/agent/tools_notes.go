package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/service"
)

// listParticipantNotesInput is the parsed input for the list_participant_notes
// tool. From/To are an optional inclusive service-date window (YYYY-MM-DD);
// both empty returns every note for the participant.
type listParticipantNotesInput struct {
	ParticipantID int64  `json:"participantId"`
	From          string `json:"from"`
	To            string `json:"to"`
}

// listParticipantNotesSchema is the model-facing JSON schema for the tool.
const listParticipantNotesSchema = `{
  "type": "object",
  "properties": {
    "participantId": { "type": "integer", "description": "Id of the participant whose notes to list." },
    "from": { "type": "string", "description": "Optional inclusive start of the service-date range (YYYY-MM-DD)." },
    "to": { "type": "string", "description": "Optional inclusive end of the service-date range (YYYY-MM-DD)." }
  },
  "required": ["participantId"],
  "additionalProperties": false
}`

// candidateView is a curated catalogue suggestion attached to a note, derived
// from the note's structured activity tag and resolved for its service date.
// The model should prefer picking a code from these over a free-form search.
type candidateView struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Unit     string   `json:"unit"`
	PriceCap *float64 `json:"priceCap"`
	GstFree  bool     `json:"gstFree"`
}

// noteView is the sanitised, model-facing projection of a note. The free-text
// Body is fenced via wrapUntrusted so the model treats it as data, never as
// instructions; the remaining fields are structured values passed through as-is.
// Candidates is a small curated set of likely NDIS codes for the note's
// activity (omitted when no catalogue is wired in).
type noteView struct {
	ID            int64           `json:"id"`
	ParticipantID int64           `json:"participantId"`
	ServiceDate   string          `json:"serviceDate"`
	Body          string          `json:"body"`
	TransportKm   *float64        `json:"transportKm"`
	SupportHours  *float64        `json:"supportHours"`
	BilledID      *int64          `json:"billedInvoiceId"`
	Candidates    []candidateView `json:"candidates,omitempty"`
}

// NewListParticipantNotesTool returns a read tool that lists a participant's
// daily journal notes within an optional service-date range. Call this when
// drafting an invoice from notes. Note bodies are free text authored by users,
// so each is returned fenced as untrusted content (the model must treat the
// body as data, not instructions).
func NewListParticipantNotesTool(notes *service.NoteService) Tool {
	return newListParticipantNotesTool(notes, nil)
}

// NewListParticipantNotesToolWithCatalog returns the same list_participant_notes
// tool but, for each note, attaches a small curated set of candidate NDIS codes
// derived from the note's structured activity tag (transportKm / supportHours)
// and resolved for the note's service date. This lets the model pick the code
// from a short list instead of free-form searching, cutting search_catalogue
// round-trips and code-mapping errors.
func NewListParticipantNotesToolWithCatalog(notes *service.NoteService, cat *service.SupportCatalogService) Tool {
	return newListParticipantNotesTool(notes, cat)
}

// newListParticipantNotesTool builds the list_participant_notes tool. When cat
// is non-nil it enriches each note with candidate catalogue codes; cat == nil
// yields the plain (no-candidates) behaviour, keeping the original constructor
// unchanged for existing callers and tests.
func newListParticipantNotesTool(notes *service.NoteService, cat *service.SupportCatalogService) Tool {
	return Tool{
		Name:        "list_participant_notes",
		Description: "List a participant's daily journal notes within an optional date range. Call this when drafting an invoice from notes.",
		Risk:        RiskRead,
		Render:      "table",
		Schema:      json.RawMessage(listParticipantNotesSchema),
		Handler: func(ctx context.Context, input json.RawMessage) (Result, error) {
			var in listParticipantNotesInput
			if err := json.Unmarshal(input, &in); err != nil {
				return Result{}, fmt.Errorf("list_participant_notes: invalid input: %w", err)
			}
			if in.ParticipantID <= 0 {
				return Result{}, fmt.Errorf("list_participant_notes: participantId must be a positive integer")
			}
			records, err := notes.ListParticipant(ctx, in.ParticipantID, in.From, in.To)
			if err != nil {
				return Result{}, fmt.Errorf("list_participant_notes: %w", err)
			}
			rows := make([]noteView, 0, len(records))
			for i := range records { // bounded by len(records)
				n := records[i]
				rows = append(rows, noteView{
					ID:            n.ID,
					ParticipantID: n.ParticipantID,
					ServiceDate:   n.ServiceDate,
					Body:          wrapUntrusted("note-body", n.Body),
					TransportKm:   n.TransportKm,
					SupportHours:  n.SupportHours,
					BilledID:      n.BilledID,
					Candidates:    noteCandidates(ctx, cat, n.ServiceDate, n.TransportKm, n.SupportHours),
				})
			}
			return Result{JSON: rows, Render: "table"}, nil
		},
	}
}

// noteCandidates resolves a small deduped set of catalogue suggestions for a
// note from its structured activity tag, for the note's service date. It is
// best-effort: cat == nil or any lookup error yields nil (no candidates) and
// never fails the read.
func noteCandidates(ctx context.Context, cat *service.SupportCatalogService, serviceDate string, transportKm, supportHours *float64) []candidateView {
	if cat == nil || serviceDate == "" {
		return nil
	}
	seen := make(map[string]struct{}, 4)
	out := make([]candidateView, 0, 4)
	add := func(query string) {
		matches, err := cat.SearchForDate(ctx, query, serviceDate, "", 3)
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
	if transportKm != nil && *transportKm > 0 {
		add("transport")
	}
	if supportHours != nil && *supportHours > 0 {
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
func NewSearchCatalogueTool(cat *service.SupportCatalogService) Tool {
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
