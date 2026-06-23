package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

// maxToolTurns bounds a propose loop's model turns. The model may call the
// read-only search_catalogue tool several times to ground its codes before it
// emits its commit tool; the final turn forces the commit tool so the loop always
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

// proposeDivide runs the divide Smart's grounding loop: the model may call the
// read-only search_catalogue tool (bounded by maxToolTurns) to find the right
// NDIS codes itself, then emits divide_session. It returns the decoded proposal.
func (s *Smarts) proposeDivide(ctx context.Context, system, userContent string) (divideSessionInput, error) {
	commit := llm.ToolDef{Name: "divide_session", Description: "Emit the line items for this session. Call exactly once, when you have resolved every code.", InputSchema: json.RawMessage(divideSessionSchema)}
	raw, err := s.proposeWithCommit(ctx, system, userContent, commit)
	if err != nil {
		return divideSessionInput{}, err
	}
	var out divideSessionInput
	if e := json.Unmarshal(raw, &out); e != nil {
		return divideSessionInput{}, fmt.Errorf("propose divide: decode divide_session: %w", e)
	}
	return out, nil
}

// proposeWithCommit drives a bounded grounding loop: the model may call the
// read-only search_catalogue tool to ground its codes, then must call the given
// commit tool exactly once. It returns the raw input of that commit tool_use.
// This is a Smart-internal loop over READ-ONLY tools plus the commit — no
// persistence, no approval gate, no conversation that outlives the call. The app
// hands the model the capability to ground itself, not a precomputed answer.
func (s *Smarts) proposeWithCommit(ctx context.Context, system, userContent string, commit llm.ToolDef) (json.RawMessage, error) {
	tools := []llm.ToolDef{
		{Name: "search_catalogue", Description: "Search the NDIS support-item catalogue for a service date. Returns matching items (code, name, unit, priceCap). Use it to find the correct code for an activity before billing it — never guess a code.", InputSchema: json.RawMessage(searchCatalogueSchema)},
		commit,
	}
	msgs := []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{{Type: llm.BlockText, Text: userContent}}}}

	for turn := 0; turn < maxToolTurns; turn++ { // bounded
		choice := llm.ToolChoice{} // auto: the model decides to search or commit
		if turn == maxToolTurns-1 {
			choice = llm.ToolChoice{ForceTool: commit.Name} // final turn: commit
		}
		resp, err := s.client.CreateMessage(ctx, llm.Request{
			System: system, Tools: tools, ToolChoice: choice, Messages: msgs,
			MaxTokens: requestMaxTokens, Model: s.cfg.Model, Effort: s.cfg.EffortFor(),
		})
		if err != nil {
			return nil, fmt.Errorf("propose: model call: %w", err)
		}
		if resp == nil {
			return nil, fmt.Errorf("propose: nil response")
		}
		uses := toolUseBlocks(resp.Content)
		if len(uses) == 0 {
			// A refusal or a truncated turn won't recover by nudging — fail fast
			// rather than burn the remaining turns.
			if resp.StopReason == llm.StopRefusal || resp.StopReason == llm.StopMaxTok {
				return nil, fmt.Errorf("propose: model stopped (%s) without committing", resp.StopReason)
			}
			// Model answered in prose; record it and nudge it back to the tools.
			msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
			msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: []llm.Block{{Type: llm.BlockText,
				Text: "Use search_catalogue to find any codes you still need, then call " + commit.Name + "."}}})
			continue
		}
		// If the model committed, return immediately — don't run searches we'd
		// discard (we terminate on the commit tool).
		for i := range uses { // bounded by len(uses)
			if uses[i].ToolName == commit.Name {
				return uses[i].Input, nil
			}
		}
		// Replay the assistant turn (text + tool_use; thinking degrades to text),
		// then answer every tool_use with a matching tool_result.
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
		results := make([]llm.ToolResult, 0, len(uses))
		for i := range uses { // bounded by len(uses)
			u := uses[i]
			if u.ToolName == "search_catalogue" {
				results = append(results, s.runSearchCatalogue(ctx, u))
				continue
			}
			results = append(results, llm.ToolResult{ToolUseID: u.ToolUseID, Content: fmt.Sprintf("unknown tool %q", u.ToolName), IsError: true})
		}
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, ToolResults: results})
	}
	return nil, fmt.Errorf("propose: model did not call %s within %d turns", commit.Name, maxToolTurns)
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
