package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

// maxToolTurns bounds the propose loop's model turns. The model may call the
// read-only search_catalogue tool several times to ground its codes before it
// emits create_invoice; the final turn forces create_invoice so the loop always
// terminates with a proposal or a clear error (rule 2).
const maxToolTurns = 8

// searchCatalogueSchema is the model-facing schema for the read-only catalogue
// search the propose loop exposes.
const searchCatalogueSchema = `{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Keywords to match against an NDIS support-item code or name, e.g. \"self-care\", \"provider travel\", \"community participation\"." },
    "serviceDate": { "type": "string", "description": "Service date (YYYY-MM-DD) to resolve the active catalogue version for." }
  },
  "required": ["query", "serviceDate"],
  "additionalProperties": false
}`

// catalogueMatchView is the trimmed search result handed back to the model.
type catalogueMatchView struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Unit     string   `json:"unit"`
	PriceCap *float64 `json:"priceCap"`
}

// proposeInvoice runs the draft Smart's grounding loop: the model may call the
// read-only search_catalogue tool (bounded by maxToolTurns) to find the right
// NDIS codes itself, then emits create_invoice. It returns the decoded proposal.
// This is a Smart-internal loop over READ-ONLY tools — no persistence, no
// approval gate, no conversation that outlives the call. The app hands the model
// the capability to ground itself, not a precomputed answer.
func (s *Smarts) proposeInvoice(ctx context.Context, system, userContent string) (createInvoiceInput, error) {
	var zero createInvoiceInput
	tools := []llm.ToolDef{
		{Name: "search_catalogue", Description: "Search the NDIS support-item catalogue for a service date. Returns matching items (code, name, unit, priceCap). Use it to find the correct code for an activity before billing it — never guess a code.", InputSchema: json.RawMessage(searchCatalogueSchema)},
		{Name: "create_invoice", Description: "Emit the final invoice covering every recorded shift. Call exactly once, when you have resolved every code.", InputSchema: json.RawMessage(createInvoiceSchema)},
	}
	msgs := []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{{Type: llm.BlockText, Text: userContent}}}}

	for turn := 0; turn < maxToolTurns; turn++ { // bounded
		choice := llm.ToolChoice{} // auto: the model decides to search or commit
		if turn == maxToolTurns-1 {
			choice = llm.ToolChoice{ForceTool: "create_invoice"} // final turn: commit
		}
		resp, err := s.client.CreateMessage(ctx, llm.Request{
			System: system, Tools: tools, ToolChoice: choice, Messages: msgs,
			MaxTokens: requestMaxTokens, Model: s.cfg.Model, Effort: s.cfg.EffortFor(),
		})
		if err != nil {
			return zero, fmt.Errorf("propose invoice: model call: %w", err)
		}
		if resp == nil {
			return zero, fmt.Errorf("propose invoice: nil response")
		}
		uses := toolUseBlocks(resp.Content)
		if len(uses) == 0 {
			// Model answered in prose; record it and nudge it back to the tools.
			msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
			msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: []llm.Block{{Type: llm.BlockText,
				Text: "Use search_catalogue to find any codes you still need, then call create_invoice."}}})
			continue
		}
		// Replay the assistant turn (text + tool_use; thinking degrades to text).
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
		results := make([]llm.ToolResult, 0, len(uses))
		for i := range uses { // bounded by len(uses)
			u := uses[i]
			if u.ToolName == "create_invoice" {
				var out createInvoiceInput
				if e := json.Unmarshal(u.Input, &out); e != nil {
					return zero, fmt.Errorf("propose invoice: decode create_invoice: %w", e)
				}
				return out, nil
			}
			if u.ToolName == "search_catalogue" {
				results = append(results, s.runSearchCatalogue(ctx, u))
				continue
			}
			results = append(results, llm.ToolResult{ToolUseID: u.ToolUseID, Content: fmt.Sprintf("unknown tool %q", u.ToolName), IsError: true})
		}
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, ToolResults: results})
	}
	return zero, fmt.Errorf("propose invoice: model did not call create_invoice within %d turns", maxToolTurns)
}

// runSearchCatalogue executes one search_catalogue tool call against the live
// catalogue and returns the result as a tool_result for the model. Read-only and
// best-effort: a bad input or lookup error comes back as an is_error result the
// model can react to, never a failure of the whole Smart. The zone is left to the
// catalogue default (national); apply-time pricing uses the tenant's real zone.
func (s *Smarts) runSearchCatalogue(ctx context.Context, u llm.Block) llm.ToolResult {
	var in struct {
		Query       string `json:"query"`
		ServiceDate string `json:"serviceDate"`
	}
	if err := json.Unmarshal(u.Input, &in); err != nil {
		return llm.ToolResult{ToolUseID: u.ToolUseID, Content: "invalid search input", IsError: true}
	}
	if in.Query == "" || in.ServiceDate == "" {
		return llm.ToolResult{ToolUseID: u.ToolUseID, Content: "query and serviceDate are required", IsError: true}
	}
	matches, err := s.catalog.SearchForDate(ctx, in.Query, in.ServiceDate, "", 8)
	if err != nil {
		return llm.ToolResult{ToolUseID: u.ToolUseID, Content: "catalogue search failed", IsError: true}
	}
	views := make([]catalogueMatchView, 0, len(matches))
	for i := range matches { // bounded by len(matches) (≤8)
		m := matches[i]
		if m == nil {
			continue
		}
		views = append(views, catalogueMatchView{Code: m.Code, Name: m.Name, Unit: m.Unit, PriceCap: m.PriceCap})
	}
	if len(views) == 0 {
		return llm.ToolResult{ToolUseID: u.ToolUseID, Content: "[]  (no matches — try different keywords)"}
	}
	body, err := json.Marshal(views)
	if err != nil {
		return llm.ToolResult{ToolUseID: u.ToolUseID, Content: "could not encode results", IsError: true}
	}
	return llm.ToolResult{ToolUseID: u.ToolUseID, Content: string(body)}
}

// toolUseBlocks returns the tool_use blocks in a model response's content.
func toolUseBlocks(content []llm.Block) []llm.Block {
	out := make([]llm.Block, 0, len(content))
	for i := range content { // bounded by len(content)
		if content[i].Type == llm.BlockToolUse {
			out = append(out, content[i])
		}
	}
	return out
}
