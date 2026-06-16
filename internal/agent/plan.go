package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// proposePlanTool is the name of the meta tool the model is forced to call in
// the plan phase. It is parsed (never executed) by the plan phase.
const proposePlanTool = "propose_plan"

// proposePlanSchema is the JSON Schema the model fills to propose a plan: an
// ordered list of {tool, summary, risk} steps.
const proposePlanSchema = `{
  "type": "object",
  "properties": {
    "steps": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "tool": { "type": "string" },
          "summary": { "type": "string" },
          "risk": { "type": "string" }
        },
        "required": ["tool", "summary", "risk"]
      }
    }
  },
  "required": ["steps"]
}`

// PlanStep is one proposed action in the plan: the tool to call, a human
// summary, and the tool's declared risk.
type PlanStep struct {
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	Risk    string `json:"risk"`
}

// planOutput is the unmarshal target for the propose_plan tool input.
type planOutput struct {
	Steps []PlanStep `json:"steps"`
}

// RegisterMetaTools registers the propose_plan meta tool on reg if it is not
// already present. The handler is a no-op: propose_plan is parsed in the plan
// phase, never executed by the registry.
func RegisterMetaTools(reg *Registry) {
	if reg == nil {
		panic("agent: RegisterMetaTools requires a non-nil registry")
	}
	if _, ok := reg.Get(proposePlanTool); ok {
		return
	}
	reg.Register(Tool{
		Name:        proposePlanTool,
		Description: "Propose an ordered plan of tool steps before acting. Always call this first.",
		Risk:        RiskMeta,
		Render:      "summary",
		Schema:      json.RawMessage(proposePlanSchema),
		Handler: func(context.Context, json.RawMessage) (Result, error) {
			return Result{}, fmt.Errorf("propose_plan is parsed in the plan phase, not executed")
		},
	})
}

// plan runs the forced propose_plan phase. It builds a request forcing
// propose_plan over the conversation's current history, calls the model,
// persists the assistant plan message, parses the proposed steps, persists each
// as a 'planned' agent_step, publishes a plan event, and returns the steps and
// the plan message id.
func (a *Agent) plan(ctx context.Context, convID, messageID int64) ([]PlanStep, int64, error) {
	if convID <= 0 || messageID <= 0 {
		return nil, 0, fmt.Errorf("plan: invalid convID=%d messageID=%d", convID, messageID)
	}

	history, err := a.loadHistory(ctx, convID)
	if err != nil {
		return nil, 0, fmt.Errorf("plan: load history: %w", err)
	}
	req := buildRequest(a.cfg, a.reg, SystemPrompt(), history, proposePlanTool)
	resp, err := a.llm.CreateMessage(ctx, req)
	if err != nil {
		return nil, 0, fmt.Errorf("plan: model call: %w", err)
	}
	if resp == nil {
		return nil, 0, fmt.Errorf("plan: nil model response")
	}

	planMsg, err := a.persistAssistant(ctx, convID, resp)
	if err != nil {
		return nil, 0, fmt.Errorf("plan: persist assistant: %w", err)
	}

	steps, err := extractPlanSteps(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("plan: %w", err)
	}

	// BUG 1: propose_plan is parsed, never executed, so its tool_use block has no
	// matching tool_result. Replaying the assistant tool_use without an answering
	// tool_result makes the real Anthropic API reject the first execute call
	// ("tool_use ids were found without tool_result blocks"). Persist a
	// tool_result keyed by the real propose_plan tool_use id (same sentinel
	// convention loadHistory/feedToolResult use) so the window stays balanced.
	planUseID := proposePlanUseID(resp)
	if planUseID == "" {
		return nil, 0, fmt.Errorf("plan: propose_plan tool_use missing id")
	}
	if e := a.feedToolResult(ctx, convID, planUseID,
		Result{JSON: map[string]string{"status": "plan recorded, proceeding"}, Render: "summary"}, false); e != nil {
		return nil, 0, fmt.Errorf("plan: persist plan tool_result: %w", e)
	}

	for i := range steps { // bounded by len(steps)
		// BUG 2: the model's free-text risk (steps[i].Risk) may not match the
		// agent_step.risk CHECK ('read','risky','meta') and would abort the
		// INSERT. Persist the AUTHORITATIVE risk derived from the registered
		// tool; unknown tools default to 'risky' (safe — gates them).
		risk := string(RiskRisky)
		if tool, ok := a.reg.Get(steps[i].Tool); ok {
			risk = string(tool.Risk)
		}
		if _, e := a.store.CreateStep(ctx, gen.CreateAgentStepParams{
			MessageID: planMsg.ID,
			Ordinal:   int64(i),
			ToolName:  steps[i].Tool,
			Summary:   steps[i].Summary,
			Risk:      risk,
			Status:    "planned",
		}); e != nil {
			return nil, 0, fmt.Errorf("plan: persist step %d: %w", i, e)
		}
	}

	a.events.Publish(convID, Event{Type: "plan", Data: steps})
	return steps, planMsg.ID, nil
}

// proposePlanUseID returns the tool_use id of the propose_plan block in resp, or
// "" if absent. Used to key the balancing tool_result for the plan turn.
func proposePlanUseID(resp *llm.Response) string {
	if resp == nil {
		return ""
	}
	for i := range resp.Content { // bounded by len(resp.Content)
		b := resp.Content[i]
		if b.Type == llm.BlockToolUse && b.ToolName == proposePlanTool {
			return b.ToolUseID
		}
	}
	return ""
}

// extractPlanSteps finds the propose_plan tool_use block in resp and unmarshals
// its steps. A missing block or empty plan is a model-protocol error.
func extractPlanSteps(resp *llm.Response) ([]PlanStep, error) {
	if resp == nil {
		return nil, fmt.Errorf("extract plan: nil response")
	}
	for i := range resp.Content { // bounded by len(resp.Content)
		b := resp.Content[i]
		if b.Type != llm.BlockToolUse || b.ToolName != proposePlanTool {
			continue
		}
		var out planOutput
		if err := json.Unmarshal(b.Input, &out); err != nil {
			return nil, fmt.Errorf("extract plan: bad input: %w", err)
		}
		if len(out.Steps) == 0 {
			return nil, fmt.Errorf("extract plan: empty plan")
		}
		return out.Steps, nil
	}
	return nil, fmt.Errorf("extract plan: no propose_plan tool_use block")
}
