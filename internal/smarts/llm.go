// Package smarts is Tallyo's curated AI layer: a small set of user-initiated,
// button-triggered "Smarts" that each gather facts, make one or a few LLM calls
// to fill a fixed schema, then deterministically validate and apply the result
// into an editable draft. There is no chat, no agent loop, and nothing the model
// produces is written without deterministic validation in between.
//
// This file is the only place that touches the Anthropic SDK. It exposes two
// entry points: Propose (one forced-single-tool call → structured fill) and
// ProposeGrounded (a bounded read-tool loop where the model searches live tenant
// data to ground specifics, then emits a final commit).
package smarts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// defaultModel is Anthropic's most capable widely-used model; overridable via
// the model arg to NewAnthropicClient (ANTHROPIC_MODEL in the composition root).
const (
	defaultModel = string(anthropic.ModelClaudeOpus4_8)
	maxTokens    = 8000
	maxToolCalls = 6 // bound on the grounded search loop (NASA rule 2)
)

// errNoCommit is returned when ProposeGrounded exhausts its bounded loop (or the
// model ends a turn) without emitting the commit tool. Callers map it to a 502.
var errNoCommit = errors.New("smarts: model did not produce a result")

// Tool is a forced-output or read-only tool definition handed to the model.
// Schema is a JSON Schema object with "properties" and optional "required".
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ProposeRequest forces a single tool call and returns its validated input JSON.
type ProposeRequest struct {
	System string
	User   string
	Force  Tool
}

// GroundedRequest lets the model call a read-only Search tool (bounded) to ground
// specifics against live data, then emit Commit. SearchFunc executes one search
// call and returns a result string fed back to the model.
type GroundedRequest struct {
	System     string
	User       string
	Search     Tool
	Commit     Tool
	SearchFunc func(ctx context.Context, input json.RawMessage) (string, error)
}

// Proposer is the boundary the Smarts depend on. Faked in tests so apply logic
// is exercised without a live API call.
type Proposer interface {
	Propose(ctx context.Context, r ProposeRequest) (json.RawMessage, error)
	ProposeGrounded(ctx context.Context, r GroundedRequest) (json.RawMessage, error)
}

// anthropicClient implements Proposer against the Anthropic Messages API.
type anthropicClient struct {
	sdk    anthropic.Client
	model  anthropic.Model
	effort string
}

// NewAnthropicClient builds the Proposer. apiKey must be non-empty (the caller
// only constructs this when ANTHROPIC_API_KEY is set). model/effort default when
// empty.
func NewAnthropicClient(apiKey, model, effort string) Proposer {
	if apiKey == "" {
		panic("smarts.NewAnthropicClient: empty api key")
	}
	m := defaultModel
	if model != "" {
		m = model
	}
	return &anthropicClient{
		sdk:    anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  anthropic.Model(m),
		effort: effort,
	}
}

// Propose makes one streaming request that MUST emit r.Force, and returns the
// tool input JSON. Adaptive thinking is intentionally NOT set: a forced
// tool_choice and adaptive thinking are mutually exclusive on the API.
func (c *anthropicClient) Propose(ctx context.Context, r ProposeRequest) (json.RawMessage, error) {
	if r.System == "" {
		return nil, fmt.Errorf("smarts: propose requires a system prompt")
	}
	if r.Force.Name == "" {
		return nil, fmt.Errorf("smarts: propose requires a forced tool")
	}
	params := c.baseParams(r.System, r.User, []Tool{r.Force}, false)
	params.ToolChoice = anthropic.ToolChoiceParamOfTool(r.Force.Name)

	msg, err := c.run(ctx, params)
	if err != nil {
		return nil, err
	}
	for _, b := range msg.Content {
		if tu, ok := b.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == r.Force.Name {
			return json.RawMessage(tu.Input), nil
		}
	}
	return nil, fmt.Errorf("smarts: model did not call %s", r.Force.Name)
}

// ProposeGrounded runs a bounded loop: the model may call Search (executed by
// r.SearchFunc) to ground specifics, then MUST emit Commit. Adaptive thinking is
// enabled here because no single tool is forced. The loop is statically bounded
// by maxToolCalls.
func (c *anthropicClient) ProposeGrounded(ctx context.Context, r GroundedRequest) (json.RawMessage, error) {
	if r.System == "" {
		return nil, fmt.Errorf("smarts: grounded propose requires a system prompt")
	}
	if r.SearchFunc == nil {
		return nil, fmt.Errorf("smarts: grounded propose requires a search func")
	}
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(r.User)),
	}
	for range maxToolCalls { // bounded loop (NASA rule 2)
		params := c.baseParams(r.System, "", []Tool{r.Search, r.Commit}, true)
		params.Messages = messages
		msg, err := c.run(ctx, params)
		if err != nil {
			return nil, err
		}

		var searches []anthropic.ToolUseBlock
		for _, b := range msg.Content {
			tu, ok := b.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}
			if tu.Name == r.Commit.Name {
				return json.RawMessage(tu.Input), nil
			}
			if tu.Name == r.Search.Name {
				searches = append(searches, tu)
			}
		}
		if len(searches) == 0 {
			// No commit, no search — re-prompting won't help.
			return nil, errNoCommit
		}

		messages = append(messages, msg.ToParam())
		results := make([]anthropic.ContentBlockParamUnion, 0, len(searches))
		for _, s := range searches {
			out, err := r.SearchFunc(ctx, json.RawMessage(s.Input))
			if err != nil {
				out = "search error: " + err.Error()
			}
			results = append(results, anthropic.NewToolResultBlock(s.ID, out, false))
		}
		messages = append(messages, anthropic.NewUserMessage(results...))
	}
	return nil, errNoCommit
}

// baseParams builds the shared request. When user is non-empty it seeds a single
// user message (Propose); grounded callers set Messages themselves. thinking
// enables adaptive thinking (only valid when no single tool is forced).
func (c *anthropicClient) baseParams(system, user string, tools []Tool, thinking bool) anthropic.MessageNewParams {
	p := anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Tools: toSDKTools(tools),
	}
	if user != "" {
		p.Messages = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user))}
	}
	if thinking {
		p.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
	}
	if c.effort != "" {
		p.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(c.effort)}
	}
	return p
}

// run streams one request and accumulates the full message (streaming is
// required once max_tokens is large with adaptive thinking).
func (c *anthropicClient) run(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	stream := c.sdk.Messages.NewStreaming(ctx, params)
	var acc anthropic.Message
	for stream.Next() { // bounded by the response event count
		if err := acc.Accumulate(stream.Current()); err != nil {
			return nil, fmt.Errorf("smarts: accumulate: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("smarts: anthropic request: %w", err)
	}
	if acc.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("smarts: request was declined")
	}
	return &acc, nil
}

// toSDKTools converts Tool defs to SDK tool unions, caching the (stable) tool
// block alongside the system prefix via an ephemeral breakpoint on the last tool.
func toSDKTools(defs []Tool) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, d := range defs {
		schema := anthropic.ToolInputSchemaParam{}
		if props, ok := d.Schema["properties"].(map[string]any); ok {
			schema.Properties = props
		}
		if req, ok := d.Schema["required"].([]string); ok {
			schema.Required = req
		}
		tool := anthropic.ToolParam{Name: d.Name, InputSchema: schema}
		if d.Description != "" {
			tool.Description = anthropic.String(d.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	if len(out) > 0 {
		if last := out[len(out)-1].OfTool; last != nil {
			last.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
	}
	return out
}
