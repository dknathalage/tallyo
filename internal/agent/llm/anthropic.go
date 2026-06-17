package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// supportsThinking reports whether model accepts the adaptive-thinking config.
// Haiku-tier models reject it (HTTP 400 "adaptive thinking is not supported on
// this model"), mirroring the effort gate in the agent package.
func supportsThinking(model string) bool {
	return !strings.Contains(model, "haiku")
}

// Anthropic implements Client.
var _ Client = (*Anthropic)(nil)

// Anthropic adapts the anthropic-sdk-go Messages API to the provider-agnostic
// Client interface. All anthropic-sdk-go symbols are confined to this file.
type Anthropic struct {
	c      anthropic.Client
	model  string
	effort string
}

// NewAnthropic builds an Anthropic client. model defaults the request model when
// Request.Model is empty; effort (one of low/medium/high/xhigh/max) sets
// output_config.effort when non-empty.
func NewAnthropic(apiKey, model, effort string) *Anthropic {
	return &Anthropic{
		c:      anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
		effort: effort,
	}
}

// CreateMessage sends one request to the Messages API and maps the result back
// into the provider-agnostic Response shape.
func (a *Anthropic) CreateMessage(ctx context.Context, req Request) (*Response, error) {
	if req.MaxTokens <= 0 {
		return nil, fmt.Errorf("llm: MaxTokens must be positive, got %d", req.MaxTokens)
	}

	model := a.model
	if req.Model != "" {
		model = req.Model
	}
	if model == "" {
		return nil, fmt.Errorf("llm: model not configured")
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  toSDKMessages(req.Messages),
	}

	// Adaptive thinking is only supported on current Opus/Fable models (Haiku-tier
	// rejects it with HTTP 400), and the API also rejects thinking + forced tool
	// use together. So enable it only on models that support it and only when no
	// tool is forced (e.g. the propose_plan phase forces a tool).
	if supportsThinking(model) && req.ToolChoice.ForceTool == "" {
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		}
	}

	if req.System != "" {
		params.System = buildSystemBlocks(req.System)
	}

	effort := a.effort
	if req.Effort != "" {
		effort = req.Effort
	}
	if effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(effort),
		}
	}

	tools, err := toSDKTools(req.Tools)
	if err != nil {
		return nil, err
	}
	if len(tools) > 0 {
		params.Tools = tools
	}

	if req.ToolChoice.ForceTool != "" {
		params.ToolChoice = anthropic.ToolChoiceParamOfTool(req.ToolChoice.ForceTool)
	}

	// Stream and accumulate. The non-streaming Messages.New is rejected by the
	// SDK ("streaming is required for operations that may take longer than 10
	// minutes") once MaxTokens is large enough with adaptive thinking, so we
	// always stream and rebuild the full Message before mapping it back.
	stream := a.c.Messages.NewStreaming(ctx, params)
	var acc anthropic.Message
	for stream.Next() { // bounded by the response event count
		event := stream.Current()
		if err := acc.Accumulate(event); err != nil {
			return nil, fmt.Errorf("llm: anthropic accumulate stream event: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("llm: anthropic create message: %w", err)
	}
	return fromSDK(&acc), nil
}

// buildSystemBlocks turns the system prompt into a single SDK text block and
// marks it with an ephemeral cache_control breakpoint. The system prompt is
// large and byte-identical every turn, so caching it lets turns 2..N re-read it
// from cache instead of paying full input price. Returns nil for an empty
// prompt (callers only set params.System when system is non-empty, but guard
// anyway).
func buildSystemBlocks(system string) []anthropic.TextBlockParam {
	if system == "" {
		return nil
	}
	blocks := []anthropic.TextBlockParam{{Text: system}}
	// Breakpoint on the last (here only) system block caches the whole prefix.
	blocks[len(blocks)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
	return blocks
}

// toSDKTools converts our ToolDef list into SDK tool unions, decoding the JSON
// Schema input into Properties/Required. The last tool carries an ephemeral
// cache_control breakpoint so the entire stable tool-definition block (every
// turn identical) is cached together with the system prefix.
func toSDKTools(defs []ToolDef) ([]anthropic.ToolUnionParam, error) {
	if len(defs) == 0 {
		return nil, nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, d := range defs {
		if d.Name == "" {
			return nil, fmt.Errorf("llm: tool with empty name")
		}
		schema := anthropic.ToolInputSchemaParam{}
		if len(d.InputSchema) > 0 {
			var parsed struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			}
			if err := json.Unmarshal(d.InputSchema, &parsed); err != nil {
				return nil, fmt.Errorf("llm: tool %q input schema: %w", d.Name, err)
			}
			schema.Properties = parsed.Properties
			schema.Required = parsed.Required
		}
		tool := anthropic.ToolParam{Name: d.Name, InputSchema: schema}
		if d.Description != "" {
			tool.Description = anthropic.String(d.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	// Breakpoint on the last tool caches all tool schemas as one block.
	if last := out[len(out)-1].OfTool; last != nil {
		last.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	return out, nil
}

// toSDKMessages converts our messages (request direction) into SDK MessageParams.
func toSDKMessages(msgs []Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(m.Content)+len(m.ToolResults))
		for _, b := range m.Content {
			switch b.Type {
			case BlockText, BlockThinking:
				blocks = append(blocks, anthropic.NewTextBlock(b.Text))
			case BlockToolUse:
				var input any
				if len(b.Input) > 0 {
					input = json.RawMessage(b.Input)
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(b.ToolUseID, input, b.ToolName))
			}
		}
		for _, tr := range m.ToolResults {
			blocks = append(blocks, anthropic.NewToolResultBlock(tr.ToolUseID, tr.Content, tr.IsError))
		}

		if m.Role == RoleAssistant {
			out = append(out, anthropic.NewAssistantMessage(blocks...))
		} else {
			out = append(out, anthropic.NewUserMessage(blocks...))
		}
	}
	return out
}

// fromSDK maps an SDK response Message back into our Response.
func fromSDK(m *anthropic.Message) *Response {
	resp := &Response{
		StopReason: mapStopReason(m.StopReason),
		Content:    make([]Block, 0, len(m.Content)),
		Usage: Usage{
			InputTokens:      m.Usage.InputTokens,
			OutputTokens:     m.Usage.OutputTokens,
			CacheReadTokens:  m.Usage.CacheReadInputTokens,
			CacheWriteTokens: m.Usage.CacheCreationInputTokens,
		},
	}
	for _, b := range m.Content {
		switch v := b.AsAny().(type) {
		case anthropic.TextBlock:
			resp.Content = append(resp.Content, Block{Type: BlockText, Text: v.Text})
		case anthropic.ThinkingBlock:
			resp.Content = append(resp.Content, Block{Type: BlockThinking, Text: v.Thinking})
		case anthropic.ToolUseBlock:
			resp.Content = append(resp.Content, Block{
				Type:      BlockToolUse,
				ToolUseID: v.ID,
				ToolName:  v.Name,
				Input:     json.RawMessage(v.Input),
			})
		}
	}
	return resp
}

// mapStopReason translates SDK stop reasons to our stop constants, passing
// unknown values through as their raw string.
func mapStopReason(r anthropic.StopReason) string {
	switch r {
	case anthropic.StopReasonRefusal:
		return StopRefusal
	case anthropic.StopReasonToolUse:
		return StopToolUse
	case anthropic.StopReasonEndTurn:
		return StopEndTurn
	case anthropic.StopReasonMaxTokens:
		return StopMaxTok
	default:
		return string(r)
	}
}
