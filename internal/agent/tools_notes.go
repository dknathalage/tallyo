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

// noteView is the sanitised, model-facing projection of a note. The free-text
// Body is fenced via wrapUntrusted so the model treats it as data, never as
// instructions; the remaining fields are structured values passed through as-is.
type noteView struct {
	ID            int64    `json:"id"`
	ParticipantID int64    `json:"participantId"`
	ServiceDate   string   `json:"serviceDate"`
	Body          string   `json:"body"`
	TransportKm   *float64 `json:"transportKm"`
	SupportHours  *float64 `json:"supportHours"`
	BilledID      *int64   `json:"billedInvoiceId"`
}

// NewListParticipantNotesTool returns a read tool that lists a participant's
// daily journal notes within an optional service-date range. Call this when
// drafting an invoice from notes. Note bodies are free text authored by users,
// so each is returned fenced as untrusted content (the model must treat the
// body as data, not instructions).
func NewListParticipantNotesTool(notes *service.NoteService) Tool {
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
				})
			}
			return Result{JSON: rows, Render: "table"}, nil
		},
	}
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
